package stream

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/rubenpari/lumen/internal/db"
)

// DiscType detects a DVD or Blu-ray folder structure (port of the VIDEO_TS/BDMV
// handling in Jellyfin's BaseVideoResolver). It returns "dvd", "bluray" or "".
func DiscType(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		n := strings.ToLower(e.Name())
		switch {
		case e.IsDir() && n == "video_ts":
			return "dvd"
		case e.IsDir() && n == "bdmv":
			return "bluray"
		case !e.IsDir() && n == "video_ts.ifo":
			return "dvd"
		case !e.IsDir() && n == "index.bdmv":
			return "bluray"
		}
	}
	return ""
}

var vobRe = regexp.MustCompile(`(?i)^VTS_(\d{2})_(\d)\.VOB$`)

// dvdVOBs returns the VOB files of the largest title set (the main feature).
func dvdVOBs(dir string) []string {
	vts := dir
	if fi, err := os.Stat(filepath.Join(dir, "VIDEO_TS")); err == nil && fi.IsDir() {
		vts = filepath.Join(dir, "VIDEO_TS")
	} else if fi, err := os.Stat(filepath.Join(dir, "video_ts")); err == nil && fi.IsDir() {
		vts = filepath.Join(dir, "video_ts")
	}
	entries, err := os.ReadDir(vts)
	if err != nil {
		return nil
	}
	sets := map[string][]string{}
	sizes := map[string]int64{}
	for _, e := range entries {
		m := vobRe.FindStringSubmatch(e.Name())
		if m == nil || m[2] == "0" { // _0.VOB is the menu
			continue
		}
		sets[m[1]] = append(sets[m[1]], filepath.Join(vts, e.Name()))
		if fi, err := e.Info(); err == nil {
			sizes[m[1]] += fi.Size()
		}
	}
	best := ""
	for k := range sets {
		if best == "" || sizes[k] > sizes[best] {
			best = k
		}
	}
	files := sets[best]
	sort.Strings(files)
	return files
}

// InputFor returns the ffmpeg/ffprobe input for an item: the file path, or a
// protocol URL for disc structures.
func InputFor(it *db.Item) string {
	switch it.DiscType {
	case "bluray":
		return "bluray:" + it.Path
	case "dvd":
		if vobs := dvdVOBs(it.Path); len(vobs) > 0 {
			return "concat:" + strings.Join(vobs, "|")
		}
	}
	return it.Path
}

// DiscSize returns the total size of the main content of a disc folder.
func DiscSize(it *db.Item) int64 {
	var total int64
	if it.DiscType == "dvd" {
		for _, v := range dvdVOBs(it.Path) {
			if fi, err := os.Stat(v); err == nil {
				total += fi.Size()
			}
		}
		return total
	}
	_ = filepath.WalkDir(it.Path, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.EqualFold(filepath.Ext(p), ".m2ts") {
			if fi, err := d.Info(); err == nil {
				total += fi.Size()
			}
		}
		return nil
	})
	return total
}
