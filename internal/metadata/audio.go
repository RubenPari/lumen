package metadata

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dhowden/tag"
)

// AudioTags are the tags read from a music file.
type AudioTags struct {
	Title       string
	Album       string
	Artist      string
	AlbumArtist string
	Genre       string
	Year        int
	Track       int
	Disc        int
	Picture     []byte
	PictureExt  string
}

// ReadAudioTags reads ID3v1/v2, MP4, FLAC and OGG tags (pure Go, no ffprobe).
func ReadAudioTags(path string) (AudioTags, error) {
	f, err := os.Open(path)
	if err != nil {
		return AudioTags{}, err
	}
	defer f.Close()
	m, err := tag.ReadFrom(f)
	if err != nil {
		return AudioTags{}, err
	}
	t := AudioTags{
		Title: strings.TrimSpace(m.Title()), Album: strings.TrimSpace(m.Album()), Artist: strings.TrimSpace(m.Artist()),
		AlbumArtist: strings.TrimSpace(m.AlbumArtist()), Genre: strings.TrimSpace(m.Genre()), Year: m.Year(),
	}
	t.Track, _ = m.Track()
	t.Disc, _ = m.Disc()
	if p := m.Picture(); p != nil && len(p.Data) > 0 {
		t.Picture = p.Data
		t.PictureExt = "." + strings.TrimPrefix(strings.ToLower(p.Ext), ".")
		if t.PictureExt == "." {
			t.PictureExt = ".jpg"
		}
	}
	return t, nil
}

// SplitArtists splits "A feat. B; C" style artist strings.
func SplitArtists(s string) []string {
	if s == "" {
		return nil
	}
	repl := strings.NewReplacer(" feat. ", ";", " ft. ", ";", " featuring ", ";", " Feat. ", ";", "/", ";", " & ", ";")
	var out []string
	seen := map[string]bool{}
	for _, p := range strings.Split(repl.Replace(s), ";") {
		p = strings.TrimSpace(p)
		if p != "" && !seen[strings.ToLower(p)] {
			seen[strings.ToLower(p)] = true
			out = append(out, p)
		}
	}
	return out
}

// SaveEmbedded writes embedded artwork to the cache and returns its path.
func SaveEmbedded(cacheDir, name string, data []byte, ext string) string {
	if len(data) == 0 {
		return ""
	}
	dst := filepath.Join(cacheDir, "embedded", name+ext)
	if fi, err := os.Stat(dst); err == nil && fi.Size() == int64(len(data)) {
		return dst
	}
	_ = os.MkdirAll(filepath.Dir(dst), 0o755)
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return ""
	}
	return dst
}

// FindAlbumArt looks for cover images in an album folder.
func FindAlbumArt(dir string) string {
	idx := indexDir(dir)
	return idx.first("cover", "folder", "front", "album", "albumart", "poster", "default")
}

// FindArtistArt looks for artist images in an artist folder.
func FindArtistArt(dir string) (primary, backdrop string) {
	idx := indexDir(dir)
	return idx.first("artist", "folder", "poster", "cover"), idx.first("fanart", "backdrop")
}

// TrackTitleFromFile strips leading track numbers: "01 - Song.mp3" -> "Song".
func TrackTitleFromFile(path string) (string, int) {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	i := 0
	for i < len(name) && name[i] >= '0' && name[i] <= '9' {
		i++
	}
	num := 0
	if i > 0 && i <= 3 {
		num, _ = strconv.Atoi(name[:i])
		rest := strings.TrimLeft(name[i:], " .-_")
		if rest != "" {
			return rest, num
		}
	}
	return name, 0
}
