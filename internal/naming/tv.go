package naming

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/dlclark/regexp2"
)

// EpisodeInfo mirrors Emby.Naming.TV.EpisodePathParserResult / EpisodeInfo.
type EpisodeInfo struct {
	Path                string
	Container           string
	SeriesName          string
	SeasonNumber        *int
	EpisodeNumber       *int
	EndingEpisodeNumber *int
	IsByDate            bool
	Year, Month, Day    int
	IsStub              bool
	Success             bool
}

func intp(v int) *int { return &v }

// ParseEpisodePath mirrors EpisodePathParser.Parse with default filters.
func ParseEpisodePath(path string, isDirectory bool, fillExtended bool) EpisodeInfo {
	if isDirectory {
		path += ".mp4"
	}
	var result *EpisodeInfo
	for _, e := range episodeExpressions {
		r := parseEpisodeExpr(path, e)
		if r.Success {
			result = &r
			break
		}
	}
	if result == nil {
		return EpisodeInfo{}
	}
	if fillExtended {
		fillAdditional(path, result)
		if result.SeriesName != "" {
			result.SeriesName = strings.TrimSpace(strings.Trim(strings.TrimSpace(result.SeriesName), "_.-"))
		}
	}
	return *result
}

func parseEpisodeExpr(name string, e *EpisodeExpression) EpisodeInfo {
	var r EpisodeInfo
	if e.IsByDate {
		name = strings.ReplaceAll(name, "_", "-")
	}
	m := match(e.regex(), name)
	if m == nil || m.GroupCount() < 3 {
		return r
	}
	switch {
	case e.IsByDate:
		full := m.String()
		for _, f := range e.DateTimeFormats {
			if t, err := time.Parse(f, full); err == nil {
				r.Year, r.Month, r.Day = t.Year(), int(t.Month()), t.Day()
				break
			}
		}
		r.Success = true
	case e.IsNamed:
		if v, ok := groupByName(m, "seasonnumber"); ok {
			if n, err := atoi32(v); err == nil {
				r.SeasonNumber = intp(n)
			}
		}
		if v, ok := groupByName(m, "epnumber"); ok {
			if n, err := atoi32(v); err == nil {
				r.EpisodeNumber = intp(n)
			}
		}
		if g := m.GroupByName("endingepnumber"); g != nil && len(g.Captures) > 0 {
			next := g.Index + g.Length
			runes := []rune(name)
			if next >= len(runes) || !strings.ContainsRune("0123456789iIpP", runes[next]) {
				if n, err := atoi32(g.String()); err == nil && r.EpisodeNumber != nil && n >= *r.EpisodeNumber {
					r.EndingEpisodeNumber = intp(n)
				}
			}
		}
		r.SeriesName, _ = groupByName(m, "seriesname")
		r.Success = r.EpisodeNumber != nil
	default:
		if v, ok := groupByNum(m, 1); ok {
			if n, err := atoi32(v); err == nil {
				r.SeasonNumber = intp(n)
			}
		}
		if v, ok := groupByNum(m, 2); ok {
			if n, err := atoi32(v); err == nil {
				r.EpisodeNumber = intp(n)
			}
		}
		r.Success = r.EpisodeNumber != nil
	}
	if s := r.SeasonNumber; s != nil && ((*s >= 200 && *s < 1928) || *s > 2500) {
		r.Success = false
	}
	r.IsByDate = e.IsByDate
	return r
}

func fillAdditional(path string, info *EpisodeInfo) {
	var exps []*EpisodeExpression
	if info.SeriesName == "" {
		for _, e := range episodeExpressions {
			if e.IsNamed {
				exps = append(exps, e)
			}
		}
	}
	exps = append(exps, multipleEpisodeExpressions...)
	for _, e := range exps {
		r := parseEpisodeExpr(path, e)
		if !r.Success {
			continue
		}
		if info.SeriesName == "" {
			info.SeriesName = r.SeriesName
		}
		if info.EndingEpisodeNumber == nil && r.EndingEpisodeNumber != nil && info.EpisodeNumber != nil &&
			*r.EndingEpisodeNumber >= *info.EpisodeNumber {
			info.EndingEpisodeNumber = r.EndingEpisodeNumber
		}
		if info.SeriesName != "" && (info.EpisodeNumber == nil || info.EndingEpisodeNumber != nil) {
			break
		}
	}
}

// ResolveEpisode mirrors EpisodeResolver.Resolve.
func ResolveEpisode(path string, isDirectory bool) *EpisodeInfo {
	isStub := false
	container := ""
	if !isDirectory {
		if !IsVideoFile(path) {
			if _, ok := ResolveStub(path); !ok {
				return nil
			}
			isStub = true
		}
		container = strings.TrimPrefix(ext(path), ".")
	}
	r := ParseEpisodePath(path, isDirectory, true)
	if !r.Success && !isStub {
		return nil
	}
	r.Path, r.Container, r.IsStub = path, container, isStub
	return &r
}

const seasonKeywordPattern = `시즌|シーズン|сезон` +
	`|season|sæson|saison|staffel|series|stagione|säsong|seizoen|seasong` +
	`|sezon|sezona|sezóna|sezonul|série|séria|serie|seria|temporada|kausi`

var (
	seasonCleanNameRe = regexp2.MustCompile(`[ ._\-\[\]]`, regexp2.None)
	seasonPreRe       = regexp2.MustCompile(`^\s*((?<seasonnumber>(?>\d+))(?:st|nd|rd|th|\.)*(?!\s*[Ee]\d+))\s*(?:`+seasonKeywordPattern+`)\s*(?<rightpart>.*)$`, regexp2.IgnoreCase)
	seasonPostRe      = regexp2.MustCompile(`^\s*(?:`+seasonKeywordPattern+`)\s*(?<seasonnumber>\d+?)(?=\d{3,4}p|[^\d]|$)(?!\s*[Ee]\d)(?<rightpart>.*)$`, regexp2.IgnoreCase)
	seasonPrefixRe    = regexp2.MustCompile(`[sS](\d{1,4})(?!\d|[eE]\d)(?=\.|_|-|\[|\]|\s|$)`, regexp2.None)
	seasonKeywordRe   = regexp2.MustCompile(seasonKeywordPattern, regexp2.IgnoreCase)
	seriesNameRe      = regexp2.MustCompile(`(?<=[^\s\._]{2})[\._]+|[\._]+(?=[^\s\._]{2})`, regexp2.None)
	titleWithYearRe   = regexp2.MustCompile(`(?<title>.+?)\s*\((?<year>[0-9]{4})\)`, regexp2.None)
)

func replaceAll(re *regexp2.Regexp, s, with string) string {
	out, err := re.Replace(s, with, -1, -1)
	if err != nil {
		return s
	}
	return out
}

// ParseSeasonPath mirrors SeasonPathParser.Parse. It returns the season number when found.
func ParseSeasonPath(path, parentPath string, supportSpecialAliases, supportNumericSeasonFolders bool) (int, bool) {
	fileName := filepath.Base(path)
	if m := match(seasonPrefixRe, fileName); m != nil {
		if v, ok := groupByNum(m, 1); ok {
			if n, err := atoi32(v); err == nil {
				return n, true
			}
		}
	}
	filename := replaceAll(seasonCleanNameRe, fileName, "")
	if parentPath != "" {
		cleanParent := replaceAll(seasonCleanNameRe, filepath.Base(parentPath), "")
		if cleanParent != "" {
			filename = replaceFold(filename, cleanParent, "")
		}
	}
	if supportSpecialAliases && (strings.EqualFold(filename, "specials") || strings.EqualFold(filename, "extras")) {
		return 0, true
	}
	if supportNumericSeasonFolders {
		if n, err := atoi32(filename); err == nil {
			return n, true
		}
	}
	isMixed := !supportNumericSeasonFolders && !supportSpecialAliases
	hasKeyword := func() bool { ok, _ := seasonKeywordRe.MatchString(fileName); return ok }
	var m = match(seasonPreRe, filename)
	if m != nil {
		if isMixed && !hasKeyword() {
			return 0, false
		}
	} else {
		m = match(seasonPostRe, filename)
		if m != nil && isMixed && !hasKeyword() {
			return 0, false
		}
	}
	if m != nil {
		if v, ok := groupByName(m, "seasonnumber"); ok {
			if n, err := atoi32(v); err == nil {
				return n, true
			}
		}
	}
	return 0, false
}

func replaceFold(s, old, with string) string {
	ls, lo := strings.ToLower(s), strings.ToLower(old)
	var b strings.Builder
	i := 0
	for {
		j := strings.Index(ls[i:], lo)
		if j < 0 {
			b.WriteString(s[i:])
			return b.String()
		}
		b.WriteString(s[i : i+j])
		b.WriteString(with)
		i += j + len(lo)
	}
}

// ParseSeriesPath mirrors SeriesPathParser.Parse.
func ParseSeriesPath(path string) (string, bool) {
	for _, e := range episodeExpressions {
		if e.IsOptimistic || !e.IsNamed {
			continue
		}
		m := match(e.regex(), path)
		if m == nil || m.GroupCount() < 3 {
			continue
		}
		season, hasSeason := groupByName(m, "seasonnumber")
		if hasSeason {
			if n, err := atoi32(season); err == nil && ((n >= 200 && n < 1928) || n > 2500) {
				continue
			}
		}
		name, _ := groupByName(m, "seriesname")
		if name != "" && season != "" {
			return strings.Trim(name, " _.-"), true
		}
	}
	return "", false
}

// SeriesInfo is the result of ResolveSeries.
type SeriesInfo struct {
	Name string
	Year int
}

// ResolveSeries mirrors SeriesResolver.Resolve.
func ResolveSeries(path string) SeriesInfo {
	name := filepath.Base(path)
	if m := match(titleWithYearRe, name); m != nil {
		t, _ := groupByName(m, "title")
		y, _ := groupByName(m, "year")
		year, _ := atoi32(y)
		return SeriesInfo{Name: strings.TrimSpace(t), Year: year}
	}
	if n, ok := ParseSeriesPath(path); ok && n != "" {
		name = n
	}
	if name != "" {
		name = strings.TrimSpace(replaceAll(seriesNameRe, name, " "))
	}
	return SeriesInfo{Name: name}
}
