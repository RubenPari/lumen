package scanner

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/rubenpari/lumen/internal/db"
	"github.com/rubenpari/lumen/internal/metadata"
	"github.com/rubenpari/lumen/internal/naming"
)

// Books libraries contain e-books/comics (one item per file) and audiobooks
// (a folder of audio files, or a single audio file, each file a chapter).
// For books, AlbumArtist holds the series name, TrackNumber the index in the
// series and DiscNumber the volume (comics).

func (ls *libScan) scanBooksDir(dir, root string) {
	if ls.ctx.Err() != nil {
		return
	}
	l := listDir(dir)
	var audio []string
	for _, f := range l.files {
		switch {
		case naming.IsBookFile(f):
			ls.buildBook(f)
		case naming.IsAudioFile(f) && !strings.EqualFold(filepath.Ext(f), ".cue"):
			audio = append(audio, f)
		}
	}
	if len(audio) > 0 {
		if dir == root {
			// Loose audio files in the root: one audiobook per file.
			for _, f := range audio {
				ls.buildAudioBook(f, []string{f})
			}
		} else {
			ls.buildAudioBook(dir, audio)
		}
	}
	for _, d := range l.dirs {
		ls.scanBooksDir(d, root)
	}
}

func (ls *libScan) buildBook(file string) {
	it, old := ls.base(db.TypeBook, file)
	info := naming.ParseBookName(noExt(filepath.Base(file)))
	it.Name = info.Name
	if it.Name == "" {
		it.Name = info.SeriesName
		if info.Index != nil {
			it.Name = fmt.Sprintf("%s #%d", info.SeriesName, *info.Index)
		}
	}
	it.AlbumArtist = info.SeriesName
	it.TrackNumber, it.DiscNumber = info.Index, info.ParentIndex
	if info.Year != nil {
		it.Year = *info.Year
	}
	unchanged := old != nil && old.Size == it.Size && old.MTime == it.MTime
	if unchanged {
		it.Name, it.Overview, it.Artists, it.Genres, it.Studios, it.ImgPrimary = old.Name, old.Overview, old.Artists, old.Genres, old.Studios, old.ImgPrimary
		it.AlbumArtist, it.TrackNumber, it.Year, it.PremiereDate = old.AlbumArtist, old.TrackNumber, old.Year, old.PremiereDate
	} else {
		m := metadata.ReadBook(file)
		if m.Title != "" {
			it.Name = m.Title
		}
		it.Artists = m.Authors
		it.Overview = m.Description
		it.Genres = m.Subjects
		if m.Publisher != "" {
			it.Studios = []string{m.Publisher}
		}
		if y := metadata.ParseDateYear(m.Date); y > 0 {
			it.Year, it.PremiereDate = y, m.Date
		}
		if m.Series != "" {
			it.AlbumArtist = m.Series
			if f, err := strconv.ParseFloat(m.SeriesIndex, 64); err == nil {
				n := int(f)
				it.TrackNumber = &n
			}
		}
		it.ImgPrimary = metadata.SaveEmbedded(ls.s.cacheDir(), it.ID, m.Cover, m.CoverExt)
	}
	li := metadata.FindFileImages(file)
	if li.Primary != "" {
		it.ImgPrimary = li.Primary
	}
	hasNFO := applyNFO(it, noExt(file)+".nfo", noExt(file)+".opf")
	mergeRemote(it, old, hasNFO)
	ls.progress(it.Name)
	if err := ls.s.DB.UpsertItem(it); err != nil {
		ls.s.Log.Error("save book", "path", file, "err", err)
	}
}

func (ls *libScan) buildAudioBook(path string, files []string) {
	type chapter struct {
		file          string
		chapter, part int
	}
	var chs []chapter
	for _, f := range files {
		r := naming.ResolveAudioBookFile(f)
		c := chapter{file: f}
		if r != nil && r.ChapterNumber != nil {
			c.chapter = *r.ChapterNumber
		}
		if r != nil && r.PartNumber != nil {
			c.part = *r.PartNumber
		}
		chs = append(chs, c)
	}
	sort.SliceStable(chs, func(i, j int) bool {
		if chs[i].part != chs[j].part {
			return chs[i].part < chs[j].part
		}
		if chs[i].chapter != chs[j].chapter {
			return chs[i].chapter < chs[j].chapter
		}
		return naturalLessPath(chs[i].file, chs[j].file)
	})
	_, mtime := fileStat(path)
	bookID := db.StableID(ls.lib.ID, db.TypeAudioBook, path)
	book := &db.Item{ID: bookID, LibraryID: ls.lib.ID, Type: db.TypeAudioBook, Path: path, IsFolder: true, MTime: mtime, DateUpdated: ls.now, DateAdded: ls.now}
	base := filepath.Base(path)
	if len(files) == 1 && files[0] == path {
		base = noExt(base)
	}
	book.Name, book.Year = naming.ParseAudioBookName(base)
	tags, _ := metadata.ReadAudioTags(chs[0].file)
	if tags.Album != "" && len(files) > 1 {
		book.Name = tags.Album
	} else if tags.Title != "" && len(files) == 1 {
		book.Name = tags.Title
	}
	if a := metadata.SplitArtists(firstNonEmpty(tags.AlbumArtist, tags.Artist)); len(a) > 0 {
		book.Artists = a
	}
	if tags.Year > 0 {
		book.Year = tags.Year
	}
	old := ls.existing[bookID]
	dir := path
	if len(files) == 1 && files[0] == path {
		dir = filepath.Dir(path)
	}
	hasNFO := false
	if dir != path {
		hasNFO = applyNFO(book, noExt(path)+".nfo")
	} else {
		hasNFO = applyNFO(book, filepath.Join(dir, "book.nfo"))
		book.ImgPrimary = metadata.FindAlbumArt(dir)
	}
	mergeRemote(book, old, hasNFO)
	if book.ImgPrimary == "" && len(tags.Picture) > 0 {
		book.ImgPrimary = metadata.SaveEmbedded(ls.s.cacheDir(), bookID, tags.Picture, tags.PictureExt)
	}
	var total float64
	var latest int64
	for i, c := range chs {
		it, oldCh := ls.base(db.TypeAudio, c.file, "chapter")
		t, _ := metadata.ReadAudioTags(c.file)
		title, _ := metadata.TrackTitleFromFile(c.file)
		if t.Title != "" && len(chs) > 1 {
			title = t.Title
		}
		n := i + 1
		it.Name, it.TrackNumber = title, &n
		it.ParentID, it.AlbumID = bookID, bookID
		it.Artists = book.Artists
		mergeRemote(it, oldCh, false)
		ls.probe(it)
		total += it.Duration
		if it.DateAdded > latest {
			latest = it.DateAdded
		}
		if err := ls.s.DB.UpsertItem(it); err != nil {
			ls.s.Log.Error("save chapter", "path", c.file, "err", err)
		}
	}
	book.Duration = total
	book.DateAdded = latest
	if old != nil {
		book.DateAdded = old.DateAdded
	}
	ls.seen[bookID] = true
	if err := ls.s.DB.UpsertItem(book); err != nil {
		ls.s.Log.Error("save audiobook", "path", path, "err", err)
	}
	ls.progress(book.Name)
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}
