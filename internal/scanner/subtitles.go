package scanner

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rubenpari/lumen/internal/db"
	"github.com/rubenpari/lumen/internal/naming"
)

// findSubtitles finds external subtitle files for a video
// (port of Emby.Naming.ExternalFiles.ExternalPathParser).
func findSubtitles(video string) []db.ExternalSub {
	dir := filepath.Dir(video)
	base := strings.ToLower(noExt(filepath.Base(video)))
	var out []db.ExternalSub
	check := func(d string, requirePrefix bool) {
		entries, err := os.ReadDir(d)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() || !naming.IsSubtitleFile(e.Name()) {
				continue
			}
			name := e.Name()
			lower := strings.ToLower(noExt(name))
			if requirePrefix && lower != base && !strings.HasPrefix(lower, base+".") && !strings.HasPrefix(lower, base+"_") && !strings.HasPrefix(lower, base+" ") {
				continue
			}
			s := db.ExternalSub{Path: filepath.Join(d, name), Format: strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")}
			rest := lower
			if requirePrefix {
				rest = strings.TrimPrefix(lower, base)
			}
			for _, tok := range strings.FieldsFunc(rest, func(r rune) bool { return r == '.' || r == '_' || r == ' ' || r == '-' || r == '[' || r == ']' }) {
				switch tok {
				case "forced", "foreign":
					s.Forced = true
				case "default":
					s.Default = true
				case "sdh", "cc", "hi":
					s.SDH = true
				default:
					if lang := normalizeLang(tok); lang != "" && s.Language == "" {
						s.Language = lang
					}
				}
			}
			out = append(out, s)
		}
	}
	check(dir, true)
	for _, sub := range []string{"subs", "subtitles", "Subs", "Subtitles"} {
		if fi, err := os.Stat(filepath.Join(dir, sub)); err == nil && fi.IsDir() {
			check(filepath.Join(dir, sub), false)
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Language < out[j].Language })
	return out
}

var langNames = map[string]string{
	"english": "eng", "italian": "ita", "italiano": "ita", "french": "fre", "german": "ger", "spanish": "spa",
	"portuguese": "por", "japanese": "jpn", "russian": "rus", "chinese": "chi", "korean": "kor", "dutch": "dut",
}

var iso1to2 = map[string]string{
	"en": "eng", "it": "ita", "fr": "fre", "de": "ger", "es": "spa", "pt": "por", "ja": "jpn", "ru": "rus",
	"zh": "chi", "ko": "kor", "nl": "dut", "sv": "swe", "no": "nor", "da": "dan", "fi": "fin", "pl": "pol",
	"tr": "tur", "el": "gre", "he": "heb", "ar": "ara", "hu": "hun", "cs": "cze", "ro": "rum", "uk": "ukr",
}

var iso2 = map[string]bool{}

func init() {
	for _, v := range iso1to2 {
		iso2[v] = true
	}
	for _, v := range []string{"fra", "deu", "zho", "nld", "ces", "ron", "ell", "slk", "slo", "hrv", "srp", "bul", "tha", "vie", "ind", "hin"} {
		iso2[v] = true
	}
}

func normalizeLang(tok string) string {
	if v, ok := iso1to2[tok]; ok {
		return v
	}
	if iso2[tok] {
		return tok
	}
	if v, ok := langNames[tok]; ok {
		return v
	}
	return ""
}
