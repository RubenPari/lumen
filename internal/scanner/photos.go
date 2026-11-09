package scanner

import (
	"path/filepath"

	"github.com/rubenpari/lumen/internal/db"
	"github.com/rubenpari/lumen/internal/metadata"
	"github.com/rubenpari/lumen/internal/naming"
)

// scanPhotoDir adds every image as a Photo; each folder with images becomes a
// PhotoAlbum (photos directly in the library root have no album).
func (ls *libScan) scanPhotoDir(dir, root string) {
	if ls.ctx.Err() != nil {
		return
	}
	l := listDir(dir)
	var photos []string
	for _, f := range l.files {
		if naming.IsPhotoFile(f) {
			photos = append(photos, f)
		}
	}
	if len(photos) > 0 {
		albumID := ""
		var album *db.Item
		if dir != root {
			albumID = db.StableID(ls.lib.ID, db.TypePhotoAlbum, dir)
			_, mtime := fileStat(dir)
			album = &db.Item{ID: albumID, LibraryID: ls.lib.ID, Type: db.TypePhotoAlbum, Name: filepath.Base(dir), Path: dir,
				IsFolder: true, MTime: mtime, DateUpdated: ls.now, DateAdded: ls.now}
			if parent := filepath.Dir(dir); parent != root {
				album.ParentID = db.StableID(ls.lib.ID, db.TypePhotoAlbum, parent)
			}
			mergeRemote(album, ls.existing[albumID], false)
			ls.seen[albumID] = true
		}
		var latest, earliest int64
		for _, f := range photos {
			it, old := ls.base(db.TypePhoto, f)
			it.Name = noExt(filepath.Base(f))
			it.ParentID, it.AlbumID = albumID, albumID
			if old != nil && old.Size == it.Size && old.MTime == it.MTime && old.Width > 0 {
				it.Width, it.Height, it.TakenAt = old.Width, old.Height, old.TakenAt
				it.Studios = old.Studios
			} else {
				pi := metadata.ReadPhoto(f)
				it.Width, it.Height, it.TakenAt = pi.Width, pi.Height, pi.TakenAt
				if pi.Camera != "" {
					it.Studios = []string{pi.Camera} // camera model
				}
			}
			if it.TakenAt == 0 {
				it.TakenAt = it.MTime
			}
			it.ImgPrimary = f
			mergeRemote(it, old, false)
			if album != nil && album.ImgPrimary == "" {
				album.ImgPrimary = f
			}
			if it.TakenAt > latest {
				latest = it.TakenAt
			}
			if earliest == 0 || it.TakenAt < earliest {
				earliest = it.TakenAt
			}
			if err := ls.s.DB.UpsertItem(it); err != nil {
				ls.s.Log.Error("save photo", "path", f, "err", err)
			}
		}
		if album != nil {
			if old := ls.existing[albumID]; old != nil {
				album.DateAdded = old.DateAdded
			}
			album.TakenAt = earliest
			_ = ls.s.DB.UpsertItem(album)
		}
		ls.progress(filepath.Base(dir))
	}
	for _, d := range l.dirs {
		ls.scanPhotoDir(d, root)
	}
}
