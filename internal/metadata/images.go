package metadata

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LocalImages are the image paths discovered next to a media item
// (port of MediaBrowser.LocalMetadata.Images.LocalImageProvider).
type LocalImages struct {
	Primary, Backdrop, Logo, Thumb string
}

var imageExts = []string{".jpg", ".jpeg", ".png", ".webp", ".tbn", ".gif"}

// dirIndex maps lower-case file names without extension to full paths.
type dirIndex map[string]string

func indexDir(dir string) dirIndex {
	idx := dirIndex{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return idx
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		ok := false
		for _, x := range imageExts {
			if x == ext {
				ok = true
				break
			}
		}
		if !ok {
			continue
		}
		key := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
		if _, exists := idx[key]; !exists {
			idx[key] = filepath.Join(dir, name)
		}
	}
	return idx
}

func (d dirIndex) first(names ...string) string {
	for _, n := range names {
		if p, ok := d[strings.ToLower(n)]; ok {
			return p
		}
	}
	return ""
}

// FindFolderImages finds images for an item that owns its folder (movie
// folder, series folder, season folder).
func FindFolderImages(dir string, kind string) LocalImages {
	idx := indexDir(dir)
	var li LocalImages
	switch kind {
	case "series":
		li.Primary = idx.first("poster", "folder", "cover", "default", "show")
	case "movie":
		li.Primary = idx.first("poster", "folder", "cover", "default", "movie")
	default:
		li.Primary = idx.first("poster", "folder", "cover", "default")
	}
	li.Backdrop = idx.first("fanart", "backdrop", "background", "art", "fanart1", "backdrop1")
	li.Logo = idx.first("logo", "clearlogo")
	li.Thumb = idx.first("landscape", "thumb")
	return li
}

// FindFileImages finds images named after a media file
// (e.g. "Movie (2010)-poster.jpg", "S01E01-thumb.jpg", "S01E01.jpg").
func FindFileImages(file string) LocalImages {
	dir := filepath.Dir(file)
	idx := indexDir(dir)
	base := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	var li LocalImages
	li.Primary = idx.first(base+"-poster", base+"-cover", base+"-folder", base)
	li.Backdrop = idx.first(base+"-fanart", base+"-backdrop", base+"-background")
	li.Logo = idx.first(base+"-logo", base+"-clearlogo")
	li.Thumb = idx.first(base+"-thumb", base+"-landscape")
	return li
}

// FindSeasonImagesInSeries finds "season01-poster.jpg" style images in the series folder.
func FindSeasonImagesInSeries(seriesDir string, season int) LocalImages {
	idx := indexDir(seriesDir)
	marker := "-specials"
	if season > 0 {
		marker = fmt.Sprintf("%02d", season)
	}
	pre := "season" + marker
	return LocalImages{
		Primary:  idx.first(pre+"-poster", pre),
		Backdrop: idx.first(pre + "-fanart"),
		Thumb:    idx.first(pre+"-landscape", pre+"-thumb"),
	}
}

// Merge fills empty fields of li with values from other.
func (li *LocalImages) Merge(other LocalImages) {
	if li.Primary == "" {
		li.Primary = other.Primary
	}
	if li.Backdrop == "" {
		li.Backdrop = other.Backdrop
	}
	if li.Logo == "" {
		li.Logo = other.Logo
	}
	if li.Thumb == "" {
		li.Thumb = other.Thumb
	}
}
