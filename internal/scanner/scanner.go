// Package scanner walks library folders and populates the database
// (a simplified port of Emby.Server.Implementations.Library resolvers).
package scanner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dlclark/regexp2"
	"github.com/rubenpari/lumen/internal/db"
	"github.com/rubenpari/lumen/internal/events"
	"github.com/rubenpari/lumen/internal/metadata"
	"github.com/rubenpari/lumen/internal/naming"
	"github.com/rubenpari/lumen/internal/stream"
)

// Status is the progress of the current scan.
type Status struct {
	Running   bool   `json:"running"`
	Library   string `json:"library,omitempty"`
	Phase     string `json:"phase,omitempty"`
	Current   int    `json:"current"`
	Total     int    `json:"total"`
	Message   string `json:"message,omitempty"`
	LastError string `json:"lastError,omitempty"`
	Finished  int64  `json:"finished,omitempty"`
}

// Scanner scans libraries.
type Scanner struct {
	DB      *db.DB
	FFprobe string
	TMDB    func() *metadata.TMDB // nil result when not configured
	Log     *slog.Logger

	Events *events.Hub

	mu       sync.Mutex
	status   Status
	lastEmit time.Time
	run      sync.Mutex
	// OnChange is invoked after a library scan completes.
	OnChange func()
}

// Status returns a copy of the current status.
func (s *Scanner) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func (s *Scanner) update(f func(*Status)) {
	s.mu.Lock()
	f(&s.status)
	st := s.status
	emit := time.Since(s.lastEmit) > 400*time.Millisecond || !st.Running
	if emit {
		s.lastEmit = time.Now()
	}
	s.mu.Unlock()
	if emit {
		s.Events.Publish(events.Event{Type: "scan.progress", Data: st})
	}
}

// ErrBusy is returned when a scan is already running.
var ErrBusy = errors.New("scan already running")

// ScanAll scans every library in the background. Returns ErrBusy if a scan is running.
func (s *Scanner) ScanAll(ctx context.Context, only ...string) error {
	if !s.run.TryLock() {
		return ErrBusy
	}
	go func() {
		defer s.run.Unlock()
		libs, err := s.DB.ListLibraries()
		if err != nil {
			s.Log.Error("list libraries", "err", err)
			return
		}
		s.update(func(st *Status) { *st = Status{Running: true} })
		var lastErr string
		for _, l := range libs {
			if len(only) > 0 && !contains(only, l.ID) {
				continue
			}
			if err := s.scanLibrary(ctx, l); err != nil {
				s.Log.Error("scan library", "library", l.Name, "err", err)
				lastErr = err.Error()
			}
		}
		s.update(func(st *Status) {
			*st = Status{Finished: time.Now().Unix(), LastError: lastErr}
		})
		if s.OnChange != nil {
			s.OnChange()
		}
	}()
	return nil
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// RefreshItem re-fetches remote metadata for a single item (and children).
func (s *Scanner) RefreshItem(ctx context.Context, id string) error {
	it, err := s.DB.GetItem(id)
	if err != nil {
		return err
	}
	target := it
	if it.Type == db.TypeSeason || it.Type == db.TypeEpisode {
		if target, err = s.DB.GetItem(it.SeriesID); err != nil {
			return err
		}
	}
	// Clear cached remote images so they are re-downloaded.
	ids := []string{target.ID}
	if target.Type == db.TypeSeries {
		kids, _ := s.DB.QueryItems(`WHERE series_id=?`, target.ID)
		for _, k := range kids {
			ids = append(ids, k.ID)
		}
	}
	for _, i := range ids {
		for _, kind := range []string{"primary", "backdrop", "logo", "thumb"} {
			matches, _ := filepath.Glob(filepath.Join(s.cacheDir(), "tmdb", i+"-"+kind+".*"))
			for _, m := range matches {
				os.Remove(m)
			}
		}
		_, _ = s.DB.Exec(`UPDATE items SET metadata_fetched=0 WHERE id=?`, i)
	}
	lib, err := s.DB.GetLibrary(target.LibraryID)
	if err != nil {
		return err
	}
	if !s.run.TryLock() {
		return ErrBusy
	}
	defer s.run.Unlock()
	// Re-run the local pass for this library so local data is reset, then fetch.
	if err := s.scanLibrary(ctx, lib); err != nil {
		return err
	}
	if s.OnChange != nil {
		s.OnChange()
	}
	return nil
}

func (s *Scanner) cacheDir() string {
	if t := s.TMDB(); t != nil {
		return t.CacheDir
	}
	return os.TempDir()
}

type libScan struct {
	s        *Scanner
	lib      *db.Library
	existing map[string]*db.Item
	seen     map[string]bool
	now      int64
	ctx      context.Context
	count    int
}

func (s *Scanner) newLibScan(ctx context.Context, lib *db.Library, where string, args ...any) (*libScan, error) {
	items, err := s.DB.QueryItems(`WHERE library_id=? `+where, append([]any{lib.ID}, args...)...)
	if err != nil {
		return nil, err
	}
	ls := &libScan{s: s, lib: lib, existing: map[string]*db.Item{}, seen: map[string]bool{}, now: db.Now(), ctx: ctx}
	for _, it := range items {
		ls.existing[it.ID] = it
	}
	return ls, nil
}

// scanRoot scans a whole library folder.
func (ls *libScan) scanRoot(root string) {
	switch ls.lib.Type {
	case "tvshows":
		ls.scanShowsRoot(root)
	default:
		ls.scanMovieDir(root, root)
	}
}

// scanUnit rescans a single top-level unit (movie folder, series folder...).
func (ls *libScan) scanUnit(unit, root string) {
	fi, err := os.Stat(unit)
	if err != nil {
		return // deleted: its items are removed by the caller
	}
	switch ls.lib.Type {
	case "tvshows":
		if fi.IsDir() && unit != root {
			ls.scanSeries(unit, root)
		} else {
			ls.scanShowsRoot(root)
		}
	default:
		if fi.IsDir() {
			ls.scanMovieDir(unit, root)
		} else {
			ls.scanMovieDir(filepath.Dir(unit), root)
		}
	}
}

// removeUnseen deletes items that were expected but not found.
func (ls *libScan) removeUnseen() int {
	removed := 0
	for id := range ls.existing {
		if !ls.seen[id] {
			if _, err := ls.s.DB.Exec(`DELETE FROM items WHERE id=?`, id); err == nil {
				removed++
			}
		}
	}
	if removed > 0 {
		ls.s.Log.Info("removed items", "library", ls.lib.Name, "count", removed)
	}
	return removed
}

func (s *Scanner) scanLibrary(ctx context.Context, lib *db.Library) error {
	s.Log.Info("scanning library", "name", lib.Name, "type", lib.Type)
	s.update(func(st *Status) {
		st.Running, st.Library, st.Phase, st.Current, st.Total, st.Message = true, lib.Name, "files", 0, 0, ""
	})
	ls, err := s.newLibScan(ctx, lib, "")
	if err != nil {
		return err
	}
	for _, root := range lib.Paths {
		if _, err := os.Stat(root); err != nil {
			s.Log.Warn("library path unavailable", "path", root, "err", err)
			// Keep items of an unavailable root (e.g. unmounted disk).
			for id, it := range ls.existing {
				if under(it.Path, root) {
					ls.seen[id] = true
				}
			}
			continue
		}
		ls.scanRoot(root)
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	ls.removeUnseen()
	lib.LastScan = db.Now()
	_ = s.DB.SaveLibrary(lib)
	if t := s.TMDB(); t != nil {
		s.fetchRemote(ctx, lib, t)
	}
	return nil
}

// under reports whether path is root or inside root.
func under(path, root string) bool {
	return path == root || strings.HasPrefix(path, strings.TrimSuffix(root, string(filepath.Separator))+string(filepath.Separator))
}

// ScanPaths rescans only the given units of a library (used by the watcher).
// Units must be absolute paths inside one of the library roots.
func (s *Scanner) ScanPaths(ctx context.Context, lib *db.Library, units []string) error {
	s.run.Lock()
	defer s.run.Unlock()
	for _, unit := range units {
		root := ""
		for _, r := range lib.Paths {
			if under(unit, r) {
				root = r
			}
		}
		if root == "" {
			continue
		}
		like := strings.ReplaceAll(strings.ReplaceAll(unit, "%", "\\%"), "_", "\\_") + string(filepath.Separator) + "%"
		ls, err := s.newLibScan(ctx, lib, `AND (path=? OR path LIKE ? ESCAPE '\')`, unit, like)
		if err != nil {
			return err
		}
		s.Log.Info("rescanning", "library", lib.Name, "path", unit)
		ls.scanUnit(unit, root)
		ls.removeUnseen()
	}
	if t := s.TMDB(); t != nil {
		s.fetchRemote(ctx, lib, t)
	}
	if s.OnChange != nil {
		s.OnChange()
	}
	return nil
}

func hidden(name string) bool {
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "@") || strings.HasPrefix(name, "#") {
		return true
	}
	switch strings.ToLower(name) {
	case "$recycle.bin", "system volume information", "lost+found", "metadata", ".trickplay":
		return true
	}
	return false
}

type dirListing struct {
	videos []string
	dirs   []string
	files  []string
}

func listDir(dir string) dirListing {
	var l dirListing
	entries, err := os.ReadDir(dir)
	if err != nil {
		return l
	}
	for _, e := range entries {
		name := e.Name()
		if hidden(name) {
			continue
		}
		p := filepath.Join(dir, name)
		isDir := e.IsDir()
		if e.Type()&os.ModeSymlink != 0 {
			if fi, err := os.Stat(p); err == nil {
				isDir = fi.IsDir()
			}
		}
		if isDir {
			l.dirs = append(l.dirs, p)
			continue
		}
		l.files = append(l.files, p)
		if naming.IsVideoFile(p) {
			l.videos = append(l.videos, p)
		}
	}
	sort.Strings(l.videos)
	sort.Strings(l.dirs)
	return l
}

var stackRules = []*regexp2.Regexp{
	regexp2.MustCompile(`^(?<filename>.*?)(?:(?<=[\]\)\}])|[ _.-]+)[\(\[]?(?<parttype>cd|dvd|part|pt|dis[ck])[ _.-]*(?<number>[0-9]+)[\)\]]?(?:\.[^.]+)?$`, regexp2.IgnoreCase),
	regexp2.MustCompile(`^(?<filename>.*?)(?:(?<=[\]\)\}])|[ _.-]+)[\(\[]?(?<parttype>cd|dvd|part|pt|dis[ck])[ _.-]*(?<number>[a-d])[\)\]]?(?:\.[^.]+)?$`, regexp2.IgnoreCase),
}

// stackKey returns the stack base name of a multi-part file ("Movie cd1.avi" -> "movie").
func stackKey(path string) string {
	name := filepath.Base(path)
	for _, re := range stackRules {
		if m, _ := re.FindStringMatch(name); m != nil {
			return strings.ToLower(m.GroupByName("filename").String())
		}
	}
	return ""
}

// collapseStacks keeps only the first part of stacked files.
func collapseStacks(videos []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, v := range videos {
		if k := stackKey(v); k != "" {
			if seen[k] {
				continue
			}
			seen[k] = true
		}
		out = append(out, v)
	}
	return out
}

func (ls *libScan) progress(msg string) {
	ls.count++
	ls.s.update(func(st *Status) { st.Current = ls.count; st.Message = msg })
}

func (ls *libScan) isExtrasDir(dir, root string) bool {
	if dir == root {
		return false
	}
	_, ok := naming.ExtrasFolderNames[strings.ToLower(filepath.Base(dir))]
	return ok
}

// scanMovieDir handles a folder inside a movie library.
func (ls *libScan) scanMovieDir(dir, root string) {
	if ls.ctx.Err() != nil || ls.isExtrasDir(dir, root) {
		return
	}
	if dir != root {
		if disc := stream.DiscType(dir); disc != "" {
			ls.buildDiscMovie(dir, root, disc)
			return
		}
	}
	l := listDir(dir)
	var main []string
	var extras []string
	for _, v := range l.videos {
		if naming.GetExtraType(v, root) != naming.ExtraNone {
			extras = append(extras, v)
			continue
		}
		main = append(main, v)
	}
	main = collapseStacks(main)
	if dir != root && len(main) == 1 {
		// Movie folder: name from the folder.
		info := naming.ResolveVideo(dir, true, true, root)
		if info.Name == "" {
			info = naming.ResolveVideo(main[0], false, true, root)
		}
		mv := ls.buildMovie(main[0], info, true)
		// Extras: trailers in the same folder or in trailers/ subfolders.
		for _, d := range l.dirs {
			if strings.EqualFold(filepath.Base(d), "trailers") {
				for _, v := range listDir(d).videos {
					extras = append(extras, v)
				}
			}
		}
		for _, x := range extras {
			if t := naming.GetExtraType(x, root); t == naming.ExtraTrailer || strings.EqualFold(filepath.Base(filepath.Dir(x)), "trailers") {
				ls.buildExtra(mv, x, string(naming.ExtraTrailer))
			}
		}
	} else {
		for _, v := range main {
			info := naming.ResolveVideo(v, false, true, root)
			ls.buildMovie(v, info, false)
		}
	}
	for _, d := range l.dirs {
		ls.scanMovieDir(d, root)
	}
}

func fileStat(path string) (int64, int64) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, 0
	}
	return fi.Size(), fi.ModTime().Unix()
}

// base builds the common part of a playable item and merges existing state.
func (ls *libScan) base(typ, file string, parts ...string) (*db.Item, *db.Item) {
	id := db.StableID(append([]string{ls.lib.ID, typ, file}, parts...)...)
	size, mtime := fileStat(file)
	it := &db.Item{ID: id, LibraryID: ls.lib.ID, Type: typ, Path: file, Size: size, MTime: mtime,
		Container: strings.TrimPrefix(strings.ToLower(filepath.Ext(file)), "."), DateAdded: mtime, DateUpdated: ls.now}
	if it.DateAdded == 0 || it.DateAdded > ls.now {
		it.DateAdded = ls.now
	}
	old := ls.existing[id]
	if old != nil {
		it.DateAdded = old.DateAdded
		if old.Size == size && old.MTime == mtime && old.Media.Probed {
			it.Media = old.Media
			it.Duration = old.Duration
		}
	}
	ls.seen[id] = true
	return it, old
}

func (ls *libScan) probe(it *db.Item) {
	if it.Media.Probed || ls.s.FFprobe == "" || strings.EqualFold(filepath.Ext(it.Path), ".strm") {
		return
	}
	mi, dur, err := metadata.Probe(ls.ctx, ls.s.FFprobe, stream.InputFor(it))
	if err != nil {
		ls.s.Log.Warn("ffprobe failed", "path", it.Path, "err", err)
		return
	}
	it.Media = mi
	if dur > 0 {
		it.Duration = dur
	}
}

// mergeRemote carries metadata previously fetched from TMDB over a freshly
// built local item when no NFO overrides it.
func mergeRemote(it, old *db.Item, hasNFO bool) {
	if old == nil {
		return
	}
	it.MetadataLocked = old.MetadataLocked
	if old.MetadataFetched == 0 && !old.MetadataLocked {
		return
	}
	it.MetadataFetched = old.MetadataFetched
	if !hasNFO || old.MetadataLocked {
		it.Name = old.Name
		it.Year = old.Year
	}
	f := func(dst *string, v string) {
		if *dst == "" {
			*dst = v
		}
	}
	f(&it.OriginalTitle, old.OriginalTitle)
	f(&it.SortName, old.SortName)
	f(&it.Overview, old.Overview)
	f(&it.Tagline, old.Tagline)
	f(&it.PremiereDate, old.PremiereDate)
	f(&it.EndDate, old.EndDate)
	f(&it.Status, old.Status)
	f(&it.OfficialRating, old.OfficialRating)
	f(&it.TmdbID, old.TmdbID)
	f(&it.ImdbID, old.ImdbID)
	f(&it.ImgPrimary, old.ImgPrimary)
	f(&it.ImgBackdrop, old.ImgBackdrop)
	f(&it.ImgLogo, old.ImgLogo)
	f(&it.ImgThumb, old.ImgThumb)
	if it.CommunityRating == 0 {
		it.CommunityRating = old.CommunityRating
	}
	if len(it.Genres) == 0 {
		it.Genres = old.Genres
	}
	if len(it.Studios) == 0 {
		it.Studios = old.Studios
	}
	if len(it.People) == 0 {
		it.People = old.People
	}
	if it.Duration == 0 {
		it.Duration = old.Duration
	}
	if old.MetadataLocked {
		it.Overview, it.Genres, it.OfficialRating, it.CommunityRating = old.Overview, old.Genres, old.OfficialRating, old.CommunityRating
		it.Tagline, it.PremiereDate, it.OriginalTitle, it.SortName = old.Tagline, old.PremiereDate, old.OriginalTitle, old.SortName
	}
}

func applyNFO(it *db.Item, candidates ...string) bool {
	for _, c := range candidates {
		if _, err := os.Stat(c); err != nil {
			continue
		}
		n, err := metadata.ReadNFO(c)
		if err != nil {
			continue
		}
		n.Apply(it)
		return true
	}
	return false
}

func noExt(p string) string { return strings.TrimSuffix(p, filepath.Ext(p)) }

func setImages(it *db.Item, li metadata.LocalImages) {
	if li.Primary != "" {
		it.ImgPrimary = li.Primary
	}
	if li.Backdrop != "" {
		it.ImgBackdrop = li.Backdrop
	}
	if li.Logo != "" {
		it.ImgLogo = li.Logo
	}
	if li.Thumb != "" {
		it.ImgThumb = li.Thumb
	}
}

func (ls *libScan) buildMovie(file string, info *naming.VideoFileInfo, ownsFolder bool) *db.Item {
	it, old := ls.base(db.TypeMovie, file)
	it.Name, it.Year = displayName(info.Name), info.Year
	if it.Name == "" {
		it.Name = noExt(filepath.Base(file))
	}
	dir := filepath.Dir(file)
	nfos := []string{noExt(file) + ".nfo"}
	li := metadata.FindFileImages(file)
	if ownsFolder {
		nfos = append(nfos, filepath.Join(dir, "movie.nfo"))
		li.Merge(metadata.FindFolderImages(dir, "movie"))
	}
	hasNFO := applyNFO(it, nfos...)
	mergeRemote(it, old, hasNFO)
	setImages(it, li)
	it.Subtitles = findSubtitles(file)
	ls.probe(it)
	ls.progress(it.Name)
	if err := ls.s.DB.UpsertItem(it); err != nil {
		ls.s.Log.Error("save movie", "path", file, "err", err)
	}
	return it
}

// buildDiscMovie adds a DVD/Blu-ray folder as a movie (played through ffmpeg).
func (ls *libScan) buildDiscMovie(dir, root, disc string) {
	it, old := ls.base(db.TypeMovie, dir)
	it.DiscType, it.Container = disc, disc
	if old != nil && old.Media.Probed && old.MTime == it.MTime {
		it.Media, it.Duration = old.Media, old.Duration
	} else {
		it.Media = db.MediaInfo{}
	}
	it.Size = stream.DiscSize(it)
	info := naming.ResolveVideo(dir, true, true, root)
	it.Name, it.Year = displayName(info.Name), info.Year
	hasNFO := applyNFO(it, filepath.Join(dir, "movie.nfo"), filepath.Join(dir, "VIDEO_TS", "VIDEO_TS.nfo"))
	mergeRemote(it, old, hasNFO)
	setImages(it, metadata.FindFolderImages(dir, "movie"))
	ls.probe(it)
	ls.progress(it.Name)
	if err := ls.s.DB.UpsertItem(it); err != nil {
		ls.s.Log.Error("save disc", "path", dir, "err", err)
	}
}

func (ls *libScan) buildExtra(parent *db.Item, file, kind string) {
	it, old := ls.base(db.TypeMovie, file, "extra")
	it.ParentID = parent.ID
	it.ExtraType = kind
	it.Name = noExt(filepath.Base(file))
	mergeRemote(it, old, false)
	ls.probe(it)
	if err := ls.s.DB.UpsertItem(it); err != nil {
		ls.s.Log.Error("save extra", "path", file, "err", err)
	}
}

// ---- TV ----

func (ls *libScan) scanShowsRoot(root string) {
	l := listDir(root)
	for _, d := range l.dirs {
		if ls.ctx.Err() != nil {
			return
		}
		ls.scanSeries(d, root)
	}
}

func (ls *libScan) scanSeries(dir, root string) {
	sid := db.StableID(ls.lib.ID, db.TypeSeries, dir)
	info := naming.ResolveSeries(dir)
	series := &db.Item{ID: sid, LibraryID: ls.lib.ID, Type: db.TypeSeries, Path: dir, IsFolder: true,
		Name: displayName(info.Name), Year: info.Year, DateUpdated: ls.now}
	_, mtime := fileStat(dir)
	series.MTime = mtime
	old := ls.existing[sid]
	hasNFO := applyNFO(series, filepath.Join(dir, "tvshow.nfo"))
	mergeRemote(series, old, hasNFO)
	setImages(series, metadata.FindFolderImages(dir, "series"))
	ls.seen[sid] = true

	type seasonAcc struct {
		item   *db.Item
		latest int64
	}
	seasons := map[int]*seasonAcc{}
	var episodes []*db.Item
	var latest int64

	var walk func(d string, folderSeason *int)
	walk = func(d string, folderSeason *int) {
		if ls.ctx.Err() != nil {
			return
		}
		l := listDir(d)
		for _, v := range l.videos {
			if naming.GetExtraType(v, root) != naming.ExtraNone {
				continue
			}
			ep := ls.buildEpisode(v, series, folderSeason)
			if ep == nil {
				continue
			}
			episodes = append(episodes, ep)
		}
		for _, sub := range l.dirs {
			if ls.isExtrasDir(sub, d) && !strings.EqualFold(filepath.Base(sub), "extras") && !strings.EqualFold(filepath.Base(sub), "specials") {
				continue
			}
			if n, ok := naming.ParseSeasonPath(sub, dir, true, true); ok {
				nn := n
				walk(sub, &nn)
			} else {
				walk(sub, folderSeason)
			}
		}
	}
	walk(dir, nil)

	// Seasons
	for _, ep := range episodes {
		sn := 1
		if ep.SeasonNumber != nil {
			sn = *ep.SeasonNumber
		}
		acc := seasons[sn]
		if acc == nil {
			seasonDir := filepath.Dir(ep.Path)
			sid2 := db.StableID(ls.lib.ID, db.TypeSeason, dir, fmt.Sprint(sn))
			name := fmt.Sprintf("Season %d", sn)
			if sn == 0 {
				name = "Specials"
			}
			num := sn
			season := &db.Item{ID: sid2, LibraryID: ls.lib.ID, Type: db.TypeSeason, ParentID: sid, SeriesID: sid,
				Name: name, Path: seasonDir, IsFolder: true, SeasonNumber: &num, DateUpdated: ls.now}
			oldSeason := ls.existing[sid2]
			li := metadata.FindSeasonImagesInSeries(dir, sn)
			if seasonDir != dir {
				applyNFO(season, filepath.Join(seasonDir, "season.nfo"))
				li.Merge(metadata.FindFolderImages(seasonDir, "season"))
			}
			mergeRemote(season, oldSeason, false)
			if oldSeason != nil && !oldSeason.MetadataLocked && oldSeason.MetadataFetched == 0 {
				season.Name = name
			}
			setImages(season, li)
			ls.seen[sid2] = true
			acc = &seasonAcc{item: season}
			seasons[sn] = acc
		}
		ep.SeasonID = acc.item.ID
		ep.ParentID = acc.item.ID
		if ep.DateAdded > acc.latest {
			acc.latest = ep.DateAdded
		}
		if ep.DateAdded > latest {
			latest = ep.DateAdded
		}
	}
	// "date added" of a series is the most recent episode.
	series.DateAdded = latest
	if series.DateAdded == 0 {
		series.DateAdded = ls.now
	}
	if err := ls.s.DB.UpsertItem(series); err != nil {
		ls.s.Log.Error("save series", "path", dir, "err", err)
		return
	}
	for _, acc := range seasons {
		acc.item.DateAdded = acc.latest
		if err := ls.s.DB.UpsertItem(acc.item); err != nil {
			ls.s.Log.Error("save season", "err", err)
		}
	}
	for _, ep := range episodes {
		if err := ls.s.DB.UpsertItem(ep); err != nil {
			ls.s.Log.Error("save episode", "path", ep.Path, "err", err)
		}
	}
	ls.progress(series.Name)
}

func (ls *libScan) buildEpisode(file string, series *db.Item, folderSeason *int) *db.Item {
	it, old := ls.base(db.TypeEpisode, file)
	it.SeriesID = series.ID
	info := naming.ResolveEpisode(file, false)
	if info != nil {
		it.SeasonNumber = info.SeasonNumber
		it.EpisodeNumber = info.EpisodeNumber
		it.EpisodeEnd = info.EndingEpisodeNumber
		if info.IsByDate && info.Year > 0 {
			it.PremiereDate = fmt.Sprintf("%04d-%02d-%02d", info.Year, info.Month, info.Day)
			it.Year = info.Year
		}
	}
	// The season parsed from the filename wins; the season folder is the fallback
	// (specials folders always win so that "Specials/S01E01" style names work).
	if folderSeason != nil && (it.SeasonNumber == nil || *folderSeason == 0) {
		v := *folderSeason
		it.SeasonNumber = &v
	}
	if it.SeasonNumber == nil {
		v := 1
		it.SeasonNumber = &v
	}
	it.Name = episodeTitle(file, series.Name, it.EpisodeNumber)
	hasNFO := applyNFO(it, noExt(file)+".nfo")
	mergeRemote(it, old, hasNFO)
	li := metadata.FindFileImages(file)
	// For episodes the "primary" image next to the file is a thumbnail.
	if li.Thumb == "" {
		li.Thumb = li.Primary
	}
	li.Primary = ""
	setImages(it, li)
	it.Subtitles = findSubtitles(file)
	ls.probe(it)
	return it
}

var episodeTitleRe = regexp2.MustCompile(`(?:[Ss]\d{1,4}[ ._-]*[Ee]\d{1,4}(?:[ ._-]*[Ee-]\d{1,4})*|\d{1,2}x\d{2,3})[ ._-]*(?<title>.*)$`, regexp2.None)

// episodeTitle derives a display title from an episode filename. When the
// filename carries no title, "Episode N" is used (localized by the client).
func episodeTitle(file, seriesName string, number *int) string {
	name := noExt(filepath.Base(file))
	fallback := func() string {
		if number != nil {
			return fmt.Sprintf("Episode %d", *number)
		}
		return displayName(name)
	}
	clean := func(t string) string {
		t = strings.TrimSpace(strings.Trim(t, " -._"))
		if c, ok := naming.CleanString(t); ok {
			t = c
		}
		if !strings.Contains(t, " ") {
			t = strings.ReplaceAll(t, ".", " ")
		}
		return strings.TrimSpace(t)
	}
	if m, _ := episodeTitleRe.FindStringMatch(name); m != nil {
		t := clean(m.GroupByName("title").String())
		if t != "" && !strings.EqualFold(t, seriesName) {
			return t
		}
		return fallback()
	}
	t := name
	if c, ok := naming.CleanString(name); ok {
		t = c
	}
	// "Series - 01 - Title" style names.
	if i := strings.LastIndex(t, " - "); i > 0 && number != nil {
		if rest := clean(t[i+3:]); rest != "" && !isDigits(rest) {
			return rest
		}
	}
	if pre := seriesName + " - "; seriesName != "" && len(t) > len(pre) && strings.EqualFold(t[:len(pre)], pre) {
		t = t[len(pre):]
	}
	t = clean(t)
	if t == "" || strings.EqualFold(t, seriesName) || isDigits(t) {
		return fallback()
	}
	return t
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// fetchRemote enriches items from TMDB for those not fetched yet.
func (s *Scanner) fetchRemote(ctx context.Context, lib *db.Library, t *metadata.TMDB) {
	typ := db.TypeMovie
	if lib.Type == "tvshows" {
		typ = db.TypeSeries
	}
	items, err := s.DB.QueryItems(`WHERE library_id=? AND type=? AND extra_type='' AND metadata_fetched=0`, lib.ID, typ)
	if err != nil || len(items) == 0 {
		return
	}
	s.update(func(st *Status) { st.Phase, st.Current, st.Total = "metadata", 0, len(items) })
	for i, it := range items {
		if ctx.Err() != nil {
			return
		}
		s.update(func(st *Status) { st.Current, st.Message = i+1, it.Name })
		var err error
		if typ == db.TypeMovie {
			err = t.FetchMovie(ctx, it)
		} else {
			err = t.FetchSeries(ctx, it)
		}
		if err != nil && !errors.Is(err, db.ErrNotFound) {
			s.Log.Warn("tmdb", "item", it.Name, "err", err)
			continue
		}
		it.MetadataFetched = db.Now()
		if err := s.DB.UpsertItem(it); err != nil {
			s.Log.Error("save", "err", err)
		}
		if typ == db.TypeSeries && it.TmdbID != "" {
			s.fetchSeasons(ctx, t, it)
		}
	}
}

func (s *Scanner) fetchSeasons(ctx context.Context, t *metadata.TMDB, series *db.Item) {
	seasons, _ := s.DB.QueryItems(`WHERE series_id=? AND type='Season'`, series.ID)
	for _, season := range seasons {
		if season.SeasonNumber == nil {
			continue
		}
		d, err := t.FetchSeason(ctx, series.TmdbID, *season.SeasonNumber)
		if err != nil {
			continue
		}
		t.ApplySeason(ctx, season, d)
		season.MetadataFetched = db.Now()
		_ = s.DB.UpsertItem(season)
		eps, _ := s.DB.QueryItems(`WHERE season_id=? AND type='Episode'`, season.ID)
		for _, ep := range eps {
			if ep.EpisodeNumber == nil {
				continue
			}
			if e, ok := d.Episodes[*ep.EpisodeNumber]; ok {
				t.ApplyEpisode(ctx, ep, e)
				ep.MetadataFetched = db.Now()
				_ = s.DB.UpsertItem(ep)
			}
		}
	}
}

// displayName makes release-style names readable: strips leading [group]
// tags and turns "Some.Movie.Name" into "Some Movie Name".
func displayName(n string) string {
	n = strings.TrimSpace(n)
	if strings.HasPrefix(n, "[") {
		if c, ok := naming.CleanString(n); ok && c != "" {
			n = c
		}
	}
	if !strings.Contains(n, " ") && strings.ContainsAny(n, "._") {
		n = strings.Join(strings.FieldsFunc(n, func(r rune) bool { return r == '.' || r == '_' }), " ")
	}
	return strings.TrimSpace(n)
}
