package naming

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/dlclark/regexp2"
)

var albumStackingPrefixes = []string{"cd", "digital media", "disc", "disk", "vol", "volume", "part", "act"}

var albumCleanRe = regexp.MustCompile(`[-\.\(\)\s]+`)

// IsMultiPartAlbumFolder mirrors AlbumParser.IsMultiPart: "CD 1", "Disc 2"...
func IsMultiPartAlbumFolder(path string) bool {
	name := filepath.Base(path)
	if name == "" || name == "." {
		return false
	}
	name = strings.TrimLeft(albumCleanRe.ReplaceAllString(name, " "), " ")
	lower := strings.ToLower(name)
	for _, p := range albumStackingPrefixes {
		if !strings.HasPrefix(lower, p) {
			continue
		}
		rest := strings.TrimSpace(name[len(p):])
		left := rest
		if i := strings.IndexByte(rest, ' '); i >= 0 {
			left = rest[:i]
		}
		if _, err := strconv.ParseInt(left, 10, 32); err == nil {
			return true
		}
	}
	return false
}

// DiscNumberFromFolder extracts N from "CD N" style folder names.
func DiscNumberFromFolder(path string) int {
	if !IsMultiPartAlbumFolder(path) {
		return 0
	}
	m := regexp.MustCompile(`\d+`).FindString(filepath.Base(path))
	n, _ := strconv.Atoi(m)
	return n
}

// BookInfo mirrors BookFileNameParserResult.
type BookInfo struct {
	Name        string
	SeriesName  string
	Index       *int
	ParentIndex *int // volume for comics
	Year        *int
}

var bookNameMatches = []*regexp2.Regexp{
	regexp2.MustCompile(`^(?<seriesName>.+?)((\s\((?<seriesYear>[0-9]{4})\))?)\s#(?<index>[0-9]+)(?:\.0)?((\s\(of\s(?<count>[0-9]+)\))?)((\s\((?<year>[0-9]{4})\))?)$`, regexp2.None),
	regexp2.MustCompile(`^(?<name>.+?)\s\((?<seriesName>.+?),\s#(?<index>[0-9]+)\)(?:\.0)?((\s\((?<year>[0-9]{4})\))?)$`, regexp2.None),
	regexp2.MustCompile(`^(?<index>[0-9]+)(?:\.0)?\s\-\s(?<name>.+?)((\s\((?<year>[0-9]{4})\))?)$`, regexp2.None),
	regexp2.MustCompile(`(?<name>.*)\((?<year>[0-9]{4})\)`, regexp2.None),
	regexp2.MustCompile(`(?<name>.*)`, regexp2.None),
}

var comicRe = regexp2.MustCompile(`^(?<name>.+?)(\sv(?<volume>[0-9]+))?(\sc(?<chapter>[0-9]+))?$`, regexp2.None)

// ParseBookName mirrors BookFileNameParser.Parse (name without extension).
func ParseBookName(name string) BookInfo {
	var r BookInfo
	for _, re := range bookNameMatches {
		m := match(re, name)
		if m == nil {
			continue
		}
		if v, ok := groupByName(m, "name"); ok {
			if cm := match(comicRe, strings.TrimSpace(v)); cm != nil {
				if vol, ok := groupByName(cm, "volume"); ok {
					if n, err := atoi32(vol); err == nil {
						r.ParentIndex = intp(n)
					}
				}
				if ch, ok := groupByName(cm, "chapter"); ok {
					if n, err := atoi32(ch); err == nil {
						r.Index = intp(n)
					}
				}
			}
			r.Name = strings.TrimSpace(v)
		}
		if v, ok := groupByName(m, "index"); ok {
			if n, err := atoi32(v); err == nil {
				r.Index = intp(n)
			}
		}
		if v, ok := groupByName(m, "year"); ok {
			if n, err := atoi32(v); err == nil {
				r.Year = intp(n)
			}
		}
		if v, ok := groupByName(m, "seriesName"); ok {
			r.SeriesName = strings.TrimSpace(v)
		}
		break
	}
	return r
}

var (
	audioBookParts = []*regexp2.Regexp{
		regexp2.MustCompile(`ch(?:apter)?[\s_-]?(?<chapter>[0-9]+)`, regexp2.IgnoreCase),
		regexp2.MustCompile(`p(?:ar)?t[\s_-]?(?<part>[0-9]+)`, regexp2.IgnoreCase),
		regexp2.MustCompile(`^(?<chapter>[0-9]+)`, regexp2.IgnoreCase),
		regexp2.MustCompile(`(?<!ch(?:apter) )(?<part>[0-9]+)$`, regexp2.IgnoreCase),
		regexp2.MustCompile(`(?<chapter>[0-9]+)_(?<part>[0-9]+)`, regexp2.IgnoreCase),
		regexp2.MustCompile(`dis(?:c|k)[\s_-]?(?<chapter>[0-9]+)`, regexp2.IgnoreCase),
	}
	audioBookNames = []*regexp2.Regexp{
		regexp2.MustCompile(`^(?<name>.+?)\s*\(\s*(?<year>[0-9]{4})\s*\)\s*$`, regexp2.IgnoreCase),
		regexp2.MustCompile(`^\s*(?<name>[^ ].*?)\s*$`, regexp2.IgnoreCase),
	}
)

// AudioBookFile mirrors AudioBookFileInfo.
type AudioBookFile struct {
	Path          string
	Container     string
	ChapterNumber *int
	PartNumber    *int
}

// ResolveAudioBookFile mirrors AudioBookResolver.Resolve.
func ResolveAudioBookFile(path string) *AudioBookFile {
	if path == "" || !IsAudioFile(path) {
		return nil
	}
	f := &AudioBookFile{Path: path, Container: strings.TrimPrefix(ext(path), ".")}
	name := fileNameWithoutExt(path)
	for _, re := range audioBookParts {
		m := match(re, name)
		if m == nil {
			continue
		}
		if f.ChapterNumber == nil {
			if v, ok := groupByName(m, "chapter"); ok {
				if n, err := atoi32(v); err == nil {
					f.ChapterNumber = intp(n)
				}
			}
		}
		if f.PartNumber == nil {
			if v, ok := groupByName(m, "part"); ok {
				if n, err := atoi32(v); err == nil {
					f.PartNumber = intp(n)
				}
			}
		}
	}
	return f
}

// ParseAudioBookName mirrors AudioBookNameParser.Parse.
func ParseAudioBookName(name string) (string, int) {
	var outName string
	year := 0
	for _, re := range audioBookNames {
		m := match(re, name)
		if m == nil {
			continue
		}
		if outName == "" {
			outName, _ = groupByName(m, "name")
		}
		if year == 0 {
			if v, ok := groupByName(m, "year"); ok {
				year, _ = atoi32(v)
			}
		}
	}
	if outName == "" {
		outName = name
	}
	return outName, year
}

var (
	photoExtensions = set(".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp", ".tif", ".tiff", ".heic", ".heif", ".avif")
	bookExtensions  = set(".epub", ".pdf", ".cbz", ".cbr", ".cb7", ".mobi", ".azw3", ".fb2")
)

// IsPhotoFile reports whether the path is an image.
func IsPhotoFile(path string) bool { _, ok := photoExtensions[ext(path)]; return ok }

// IsBookFile reports whether the path is an e-book or comic.
func IsBookFile(path string) bool { _, ok := bookExtensions[ext(path)]; return ok }
