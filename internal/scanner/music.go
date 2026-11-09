package scanner

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rubenpari/lumen/internal/db"
	"github.com/rubenpari/lumen/internal/metadata"
	"github.com/rubenpari/lumen/internal/naming"
)

// Music library layout (like Jellyfin): every folder that contains audio
// files is an album; "CD 1"/"Disc 2" subfolders belong to their parent album.
// Artists are derived from the album artist tag (falling back to the parent
// folder name for Artist/Album layouts).

type trackScan struct {
	path string
	tags metadata.AudioTags
	disc int
}

func (ls *libScan) scanMusicDir(dir, root string) {
	if ls.ctx.Err() != nil {
		return
	}
	l := listDir(dir)
	var tracks []trackScan
	collect := func(d string, disc int) {
		for _, f := range listDir(d).files {
			if !naming.IsAudioFile(f) || strings.EqualFold(filepath.Ext(f), ".cue") {
				continue
			}
			t, _ := metadata.ReadAudioTags(f)
			if disc > 0 && t.Disc == 0 {
				t.Disc = disc
			}
			tracks = append(tracks, trackScan{path: f, tags: t, disc: t.Disc})
		}
	}
	collect(dir, 0)
	var subdirs []string
	for _, d := range l.dirs {
		if naming.IsMultiPartAlbumFolder(d) {
			collect(d, naming.DiscNumberFromFolder(d))
		} else {
			subdirs = append(subdirs, d)
		}
	}
	if len(tracks) > 0 {
		ls.buildAlbum(dir, root, tracks)
	}
	for _, d := range subdirs {
		ls.scanMusicDir(d, root)
	}
}

func mostCommon(values []string) string {
	count := map[string]int{}
	best, bestN := "", 0
	for _, v := range values {
		if v == "" {
			continue
		}
		count[v]++
		if count[v] > bestN {
			best, bestN = v, count[v]
		}
	}
	return best
}

func (ls *libScan) artist(name, folder string) *db.Item {
	if name == "" {
		name = "Unknown Artist"
	}
	id := db.StableID(ls.lib.ID, db.TypeMusicArtist, strings.ToLower(name))
	path := "artist:" + name
	it := &db.Item{ID: id, LibraryID: ls.lib.ID, Type: db.TypeMusicArtist, Name: name, Path: path, IsFolder: true, DateUpdated: ls.now, DateAdded: ls.now}
	if old := ls.existing[id]; old != nil {
		it.DateAdded = old.DateAdded
		mergeRemote(it, old, false)
	} else if prev, err := ls.s.DB.GetItem(id); err == nil {
		it.DateAdded = prev.DateAdded
		mergeRemote(it, prev, false)
	}
	if folder != "" && strings.EqualFold(filepath.Base(folder), name) {
		it.Path = folder
		p, b := metadata.FindArtistArt(folder)
		setImages(it, metadata.LocalImages{Primary: p, Backdrop: b})
		applyNFO(it, filepath.Join(folder, "artist.nfo"))
		it.Name = name
	}
	ls.seen[id] = true
	return it
}

func (ls *libScan) buildAlbum(dir, root string, tracks []trackScan) {
	sort.SliceStable(tracks, func(i, j int) bool {
		a, b := tracks[i], tracks[j]
		if a.disc != b.disc {
			return a.disc < b.disc
		}
		if a.tags.Track != b.tags.Track {
			return a.tags.Track < b.tags.Track
		}
		return naturalLessPath(a.path, b.path)
	})
	var albumNames, albumArtists, artists, genres []string
	year := 0
	for _, t := range tracks {
		albumNames = append(albumNames, t.tags.Album)
		albumArtists = append(albumArtists, t.tags.AlbumArtist)
		artists = append(artists, t.tags.Artist)
		if t.tags.Genre != "" {
			genres = append(genres, t.tags.Genre)
		}
		if t.tags.Year > year {
			year = t.tags.Year
		}
	}
	albumName := mostCommon(albumNames)
	folderName, folderYear := naming.CleanDateTime(filepath.Base(dir))
	if albumName == "" {
		albumName = folderName
	}
	if year == 0 {
		year = folderYear
	}
	albumArtist := mostCommon(albumArtists)
	if albumArtist == "" {
		if a := metadata.SplitArtists(mostCommon(artists)); len(a) > 0 {
			albumArtist = a[0]
		}
	}
	parent := filepath.Dir(dir)
	if albumArtist == "" && parent != root {
		albumArtist = filepath.Base(parent)
	}
	artistFolder := ""
	if parent != root {
		artistFolder = parent
	}
	artist := ls.artist(albumArtist, artistFolder)

	_, mtime := fileStat(dir)
	albumID := db.StableID(ls.lib.ID, db.TypeMusicAlbum, dir)
	album := &db.Item{ID: albumID, LibraryID: ls.lib.ID, Type: db.TypeMusicAlbum, Name: albumName, Path: dir, IsFolder: true,
		Year: year, ParentID: artist.ID, ArtistID: artist.ID, AlbumArtist: artist.Name, MTime: mtime, DateUpdated: ls.now, DateAdded: ls.now}
	oldAlbum := ls.existing[albumID]
	hasNFO := applyNFO(album, filepath.Join(dir, "album.nfo"))
	mergeRemote(album, oldAlbum, hasNFO)
	if len(album.Genres) == 0 {
		seen := map[string]bool{}
		for _, g := range genres {
			for _, p := range strings.Split(g, ";") {
				if p = strings.TrimSpace(p); p != "" && !seen[p] {
					seen[p] = true
					album.Genres = append(album.Genres, p)
				}
			}
		}
	}
	album.ImgPrimary = metadata.FindAlbumArt(dir)
	if album.ImgPrimary == "" {
		for _, t := range tracks {
			if len(t.tags.Picture) > 0 {
				album.ImgPrimary = metadata.SaveEmbedded(ls.s.cacheDir(), albumID, t.tags.Picture, t.tags.PictureExt)
				break
			}
		}
	}
	if artist.ImgPrimary == "" {
		artist.ImgThumb = album.ImgPrimary // used as a fallback image for the artist
	}

	var latest int64
	var total float64
	for i, t := range tracks {
		it, old := ls.base(db.TypeAudio, t.path)
		title, num := metadata.TrackTitleFromFile(t.path)
		if t.tags.Title != "" {
			title = t.tags.Title
		}
		track := t.tags.Track
		if track == 0 {
			track = num
		}
		if track == 0 {
			track = i + 1
		}
		it.Name, it.Year = title, year
		it.AlbumID, it.ParentID, it.ArtistID, it.AlbumArtist = albumID, albumID, artist.ID, artist.Name
		it.Artists = metadata.SplitArtists(t.tags.Artist)
		if len(it.Artists) == 0 {
			it.Artists = []string{artist.Name}
		}
		it.TrackNumber = &track
		if t.disc > 0 {
			d := t.disc
			it.DiscNumber = &d
		}
		it.Genres = album.Genres
		mergeRemote(it, old, false)
		if old != nil && old.MetadataFetched == 0 {
			it.Name = title
		}
		ls.probe(it)
		total += it.Duration
		if it.DateAdded > latest {
			latest = it.DateAdded
		}
		if err := ls.s.DB.UpsertItem(it); err != nil {
			ls.s.Log.Error("save track", "path", t.path, "err", err)
		}
	}
	album.Duration = total
	album.DateAdded = latest
	if oldAlbum != nil && oldAlbum.DateAdded < latest {
		album.DateAdded = oldAlbum.DateAdded
	}
	if artist.DateAdded == 0 || artist.DateAdded < latest {
		artist.DateAdded = latest
	}
	ls.seen[albumID] = true
	_ = ls.s.DB.UpsertItem(artist)
	if err := ls.s.DB.UpsertItem(album); err != nil {
		ls.s.Log.Error("save album", "path", dir, "err", err)
	}
	ls.progress(fmt.Sprintf("%s — %s", artist.Name, album.Name))
}

func naturalLessPath(a, b string) bool {
	return metadata.NaturalLess(filepath.Base(a), filepath.Base(b))
}

// pruneArtists removes artists without albums (artists are not tied to a single folder).
func (s *Scanner) pruneArtists(libID string) {
	_, _ = s.DB.Exec(`DELETE FROM items WHERE library_id=? AND type='MusicArtist' AND NOT EXISTS (SELECT 1 FROM items a WHERE a.artist_id=items.id AND a.type='MusicAlbum')`, libID)
}
