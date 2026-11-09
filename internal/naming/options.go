// Package naming is a Go port of Jellyfin's Emby.Naming library. The regular
// expressions are kept in .NET syntax and evaluated with regexp2 so that the
// behaviour matches the original implementation as closely as possible.
package naming

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dlclark/regexp2"
)

// ExtraType identifies a kind of extra content (trailers, featurettes...).
type ExtraType string

const (
	ExtraNone            ExtraType = ""
	ExtraUnknown         ExtraType = "Unknown"
	ExtraTrailer         ExtraType = "Trailer"
	ExtraThemeSong       ExtraType = "ThemeSong"
	ExtraThemeVideo      ExtraType = "ThemeVideo"
	ExtraBehindTheScenes ExtraType = "BehindTheScenes"
	ExtraDeletedScene    ExtraType = "DeletedScene"
	ExtraInterview       ExtraType = "Interview"
	ExtraScene           ExtraType = "Scene"
	ExtraSample          ExtraType = "Sample"
	ExtraShort           ExtraType = "Short"
	ExtraFeaturette      ExtraType = "Featurette"
	ExtraClip            ExtraType = "Clip"
)

type extraRuleType int

const (
	ruleFilename extraRuleType = iota
	ruleSuffix
	ruleDirectoryName
)

type mediaKind int

const (
	kindVideo mediaKind = iota
	kindAudio
)

type extraRule struct {
	Type      ExtraType
	RuleType  extraRuleType
	Token     string
	MediaKind mediaKind
}

// EpisodeExpression mirrors Emby.Naming.Common.EpisodeExpression.
type EpisodeExpression struct {
	Expression                  string
	IsByDate                    bool
	IsOptimistic                bool
	IsNamed                     bool
	SupportsAbsoluteEpisodeNums bool
	DateTimeFormats             []string
	re                          *regexp2.Regexp
}

func (e *EpisodeExpression) regex() *regexp2.Regexp {
	if e.re == nil {
		e.re = regexp2.MustCompile(e.Expression, regexp2.IgnoreCase)
	}
	return e.re
}

func set(items ...string) map[string]struct{} {
	m := make(map[string]struct{}, len(items))
	for _, i := range items {
		m[i] = struct{}{}
	}
	return m
}

var (
	videoExtensions = set(".001", ".3g2", ".3gp", ".amv", ".asf", ".asx", ".avi", ".bin", ".bivx", ".divx", ".dv", ".dvr-ms",
		".f4v", ".fli", ".flv", ".ifo", ".img", ".iso", ".m2t", ".m2ts", ".m2v", ".m4v", ".mkv", ".mk3d", ".mov", ".mp4",
		".mpe", ".mpeg", ".mpg", ".mts", ".mxf", ".nrg", ".nsv", ".nuv", ".ogm", ".ogv", ".pva", ".qt", ".rec", ".rm",
		".rmvb", ".strm", ".svq3", ".tp", ".ts", ".ty", ".viv", ".vob", ".vp3", ".webm", ".wmv", ".wtv", ".xvid")

	audioExtensions = set(".669", ".3gp", ".aa", ".aac", ".aax", ".ac3", ".act", ".adp", ".adplug", ".adx", ".afc", ".amf",
		".aif", ".aifc", ".aiff", ".alac", ".amr", ".ape", ".ast", ".au", ".awb", ".cda", ".cue", ".dmf", ".dsf", ".dsm",
		".dsp", ".dts", ".dvf", ".eac3", ".ec3", ".far", ".flac", ".gdm", ".gsm", ".gym", ".hps", ".imf", ".it", ".m15",
		".m4a", ".m4b", ".mac", ".med", ".mka", ".mmf", ".mod", ".mogg", ".mp2", ".mp3", ".mpa", ".mpc", ".mpp", ".mp+",
		".msv", ".nmf", ".nsf", ".nsv", ".oga", ".ogg", ".okt", ".opus", ".pls", ".ra", ".rf64", ".rm", ".s3m", ".sfx",
		".shn", ".sid", ".stm", ".strm", ".ult", ".uni", ".vox", ".wav", ".wma", ".wv", ".xm", ".xsp", ".ymf")

	subtitleExtensions = set(".ass", ".mks", ".sami", ".smi", ".srt", ".ssa", ".sub", ".sup", ".vtt")

	stubExtensions = set(".disc")

	stubTypes = []struct{ Type, Token string }{
		{"dvd", "dvd"}, {"hddvd", "hddvd"}, {"bluray", "bluray"}, {"bluray", "brrip"}, {"bluray", "bd25"},
		{"bluray", "bd50"}, {"vhs", "vhs"}, {"tv", "HDTV"}, {"tv", "PDTV"}, {"tv", "DSR"},
	}

	videoFlagDelimiters = "()-._[]"

	cleanDateTimeRegexes = compileAll(
		`(.+[^_\,\.\(\)\[\]\-])[_\.\(\)\[\]\-](19[0-9]{2}|20[0-9]{2})(?![0-9]+|\W[0-9]{2}\W[0-9]{2})([ _\,\.\(\)\[\]\-][^0-9]|).*(19[0-9]{2}|20[0-9]{2})*`,
		`(.+[^_\,\.\(\)\[\]\-])[ _\.\(\)\[\]\-]+(19[0-9]{2}|20[0-9]{2})(?![0-9]+|\W[0-9]{2}\W[0-9]{2})([ _\,\.\(\)\[\]\-][^0-9]|).*(19[0-9]{2}|20[0-9]{2})*`,
	)

	cleanStringRegexes = compileAll(
		`^\s*(?<cleaned>.+?)[ _\,\.\(\)\[\]\-](3d|sbs|tab|hsbs|htab|mvc|HDR|HDC|UHD|UltraHD|4k|ac3|dts|custom|dc|divx|divx5|dsr|dsrip|dutch|dvd|dvdrip|dvdscr|dvdscreener|screener|dvdivx|cam|fragment|fs|hdtv|hdrip|hdtvrip|internal|limited|multi|subs|ntsc|ogg|ogm|pal|pdtv|proper|repack|rerip|retail|cd[1-9]|r5|bd5|bd|se|svcd|swedish|german|read.nfo|nfofix|unrated|ws|web-dl|telesync|ts|telecine|tc|brrip|bdrip|480p|480i|576p|576i|720p|720i|1080p|1080i|2160p|hrhd|hrhdtv|hddvd|bluray|blu-ray|x264|x265|h264|h265|xvid|xvidvd|xxx|www.www|AAC|DTS)(?=[ _\,\.\(\)\[\]\-]|$)`,
		`^\s*(?<cleaned>.+?)((\s*\[[^\]]+\]\s*)+)(\.[^\s]+)?$`,
		`^\s*(?<cleaned>.+?)\WE[0-9]+(-|~)E?[0-9]+(\W|$)`,
		`^\s*\[[^\]]+\](?!\.\w+$)\s*(?<cleaned>.+)`,
		`^\s*(?<cleaned>.+?)\s+-\s+[0-9]+\s*$`,
		`^\s*(?<cleaned>.+?)(([-._ ](trailer|sample))|-(scene|clip|behindthescenes|deleted|deletedscene|featurette|short|interview|other|extra))$`,
	)

	format3DRules = []struct{ Preceding, Token string }{
		{"3d", "hsbs"}, {"3d", "sbs"}, {"3d", "htab"}, {"3d", "tab"},
		{"", "fsbs"}, {"", "hsbs"}, {"", "sbs"}, {"", "ftab"}, {"", "htab"}, {"", "tab"}, {"", "sbs3d"}, {"", "mvc"},
	}

	videoExtraRules = []extraRule{
		{ExtraTrailer, ruleDirectoryName, "trailers", kindVideo},
		{ExtraThemeVideo, ruleDirectoryName, "backdrops", kindVideo},
		{ExtraThemeSong, ruleDirectoryName, "theme-music", kindAudio},
		{ExtraBehindTheScenes, ruleDirectoryName, "behind the scenes", kindVideo},
		{ExtraDeletedScene, ruleDirectoryName, "deleted scenes", kindVideo},
		{ExtraInterview, ruleDirectoryName, "interviews", kindVideo},
		{ExtraScene, ruleDirectoryName, "scenes", kindVideo},
		{ExtraSample, ruleDirectoryName, "samples", kindVideo},
		{ExtraSample, ruleDirectoryName, "sample", kindVideo},
		{ExtraShort, ruleDirectoryName, "shorts", kindVideo},
		{ExtraFeaturette, ruleDirectoryName, "featurettes", kindVideo},
		{ExtraUnknown, ruleDirectoryName, "extras", kindVideo},
		{ExtraUnknown, ruleDirectoryName, "extra", kindVideo},
		{ExtraUnknown, ruleDirectoryName, "other", kindVideo},
		{ExtraClip, ruleDirectoryName, "clips", kindVideo},
		{ExtraTrailer, ruleFilename, "trailer", kindVideo},
		{ExtraSample, ruleFilename, "sample", kindVideo},
		{ExtraThemeSong, ruleFilename, "theme", kindAudio},
		{ExtraTrailer, ruleSuffix, "-trailer", kindVideo},
		{ExtraTrailer, ruleSuffix, ".trailer", kindVideo},
		{ExtraTrailer, ruleSuffix, "_trailer", kindVideo},
		{ExtraTrailer, ruleSuffix, "- trailer", kindVideo},
		{ExtraSample, ruleSuffix, "-sample", kindVideo},
		{ExtraSample, ruleSuffix, ".sample", kindVideo},
		{ExtraSample, ruleSuffix, "_sample", kindVideo},
		{ExtraSample, ruleSuffix, "- sample", kindVideo},
		{ExtraScene, ruleSuffix, "-scene", kindVideo},
		{ExtraClip, ruleSuffix, "-clip", kindVideo},
		{ExtraInterview, ruleSuffix, "-interview", kindVideo},
		{ExtraBehindTheScenes, ruleSuffix, "-behindthescenes", kindVideo},
		{ExtraDeletedScene, ruleSuffix, "-deleted", kindVideo},
		{ExtraDeletedScene, ruleSuffix, "-deletedscene", kindVideo},
		{ExtraFeaturette, ruleSuffix, "-featurette", kindVideo},
		{ExtraShort, ruleSuffix, "-short", kindVideo},
		{ExtraUnknown, ruleSuffix, "-extra", kindVideo},
		{ExtraUnknown, ruleSuffix, "-other", kindVideo},
	}

	// ExtrasFolderNames maps folder names (lower case) that contain extras.
	ExtrasFolderNames = func() map[string]ExtraType {
		m := map[string]ExtraType{}
		for _, r := range videoExtraRules {
			if r.RuleType == ruleDirectoryName {
				m[r.Token] = r.Type
			}
		}
		return m
	}()
)

var episodeExpressions = []*EpisodeExpression{
	{Expression: `.*(\\|\/)(?<seriesname>((?![Ss]([0-9]+)[][ ._-]*[Ee]([0-9]+))[^\\\/])*)?[Ss](?<seasonnumber>[0-9]+)[][ ._-]*[Ee](?<epnumber>[0-9]+)([^\\/]*)$`, IsNamed: true},
	{Expression: `[\._ -]()[Ee][Pp]_?([0-9]+)([^\\/]*)$`},
	{Expression: `[^\\/]*?()\.?[Ee]([0-9]+)\.([^\\/]*)$`},
	{Expression: `(?<year>[0-9]{4})[._ -](?<month>[0-9]{2})[._ -](?<day>[0-9]{2})`, IsByDate: true, DateTimeFormats: []string{"2006.01.02", "2006-01-02", "2006_01_02", "2006 01 02"}},
	{Expression: `(?<day>[0-9]{2})[._ -](?<month>[0-9]{2})[._ -](?<year>[0-9]{4})`, IsByDate: true, DateTimeFormats: []string{"02.01.2006", "02-01-2006", "02_01_2006", "02 01 2006"}},
	{Expression: `.*[\\\/]((?<seriesname>[^\\/]+?)\s)?[Ss](?:eason)?\s*(?<seasonnumber>[0-9]+)\s+[Ee](?:pisode)?\s*(?<epnumber>[0-9]+).*$`, IsNamed: true},
	{Expression: `.*[\\\/](?![Ee]pisode)(?![^\\\/]*[Ss][0-9]+[][ ._-]*[Ee][0-9]+)(?<seriesname>[\w\s]+?)\s(?<epnumber>[0-9]{1,4})(-(?<endingepnumber>[0-9]{2,4}))*[^\\\/x]*$`, IsNamed: true},
	{Expression: `[\\\/\._ \[\(-]([0-9]+)x([0-9]+(?:(?:[a-i]|\.[1-9])(?![0-9]))?)([^\\\/]*)$`, SupportsAbsoluteEpisodeNums: true},
	{Expression: `.*[\\\/]?.*?(\[.*?\])+.*?(?<seriesname>[-\w\s]+?)[\s_]*-[\s_]*(?<epnumber>[0-9]+).*$`, IsNamed: true},
	{Expression: `.*[\\\/](?<seriesname>[^\\\/]+?)[\s_]+-[\s_]+(?<epnumber>[0-9]+)[\s_]*(?:\[.*?\]|\(.*?\))*[\s_]*(?:\.\w+)?$`, IsNamed: true},
	{Expression: `[\\/._ -](?<seriesname>(?![0-9]+[0-9][0-9])([^\\\/_])*)[\\\/._ -](?<seasonnumber>[0-9]+)(?<epnumber>[0-9][0-9](?:(?:[a-i]|\.[1-9])(?![0-9]))?)([._ -][^\\\/]*)$`, IsOptimistic: true, IsNamed: true},
	{Expression: `[\/._ -]p(?:ar)?t[_. -]()([ivx]+|[0-9]+)([._ -][^\/]*)$`, SupportsAbsoluteEpisodeNums: true},
	{Expression: `[Ee]pisode (?<epnumber>[0-9]+)(-(?<endingepnumber>[0-9]+))?[^\\\/]*$`, IsNamed: true},
	{Expression: `.*(\\|\/)[sS]?(?<seasonnumber>[0-9]+)[xX](?<epnumber>[0-9]+)[^\\\/]*$`, IsNamed: true},
	{Expression: `.*(\\|\/)[sS](?<seasonnumber>[0-9]+)[x,X]?[eE](?<epnumber>[0-9]+)[^\\\/]*$`, IsNamed: true},
	{Expression: `.*(\\|\/)(?<seriesname>((?![sS]?[0-9]{1,4}[xX][0-9]{1,3})[^\\\/])*)?([sS]?(?<seasonnumber>[0-9]{1,4})[xX](?<epnumber>[0-9]+))[^\\\/]*$`, IsNamed: true},
	{Expression: `.*(\\|\/)(?<seriesname>[^\\\/]*)[sS](?<seasonnumber>[0-9]{1,4})[xX\.]?[eE](?<epnumber>[0-9]+)[^\\\/]*$`, IsNamed: true},
	{Expression: `.*[\\\/](?<epnumber>[0-9]+)(-(?<endingepnumber>[0-9]+))*\.\w+$`, IsOptimistic: true, IsNamed: true},
	{Expression: `([0-9]+)-([0-9]+)`},
	{Expression: `.*(\\|\/)(?<epnumber>[0-9]{1,3})(-(?<endingepnumber>[0-9]{2,3}))*\s?-\s?[^\\\/]*$`, IsOptimistic: true, IsNamed: true},
	{Expression: `.*(\\|\/)(?<epnumber>[0-9]{1,3})(-(?<endingepnumber>[0-9]{2,3}))*\.[^\\\/]+$`, IsOptimistic: true, IsNamed: true},
	{Expression: `.*[\\\/][^\\\/]* - (?<epnumber>[0-9]{1,3})(-(?<endingepnumber>[0-9]{2,3}))*[^\\\/]*$`, IsOptimistic: true, IsNamed: true},
	{Expression: `[Ss]eason[\._ ](?<seasonnumber>[0-9]+)[\\\/](?<epnumber>[0-9]{1,3})([^\\\/]*)$`, IsOptimistic: true, IsNamed: true},
	{Expression: `(.*(\\|\/))*(?<seriesname>.+)\/[Ss](eason)?[\. _\-]*(?<seasonnumber>[0-9]+)`, IsNamed: true},
	{Expression: `(.*(\\|\/))*(?<seriesname>.+)[\. _\-]+[sS](eason)?[\. _\-]*(?<seasonnumber>[0-9]+)`, IsNamed: true},
	{Expression: `(?:\[(?:[^\]]+)\]\s*)?(?<seriesname>\[[^\]]+\]|[^[\]]+)\s*\[(?<epnumber>[0-9]+)\]`, IsNamed: true},
}

var multipleEpisodeExpressions = func() []*EpisodeExpression {
	exps := []string{
		`.*(\\|\/)[sS]?(?<seasonnumber>[0-9]{1,4})[xX](?<epnumber>[0-9]{1,3})((-| - )[0-9]{1,4}[eExX](?<endingepnumber>[0-9]{1,3}))+[^\\\/]*$`,
		`.*(\\|\/)[sS]?(?<seasonnumber>[0-9]{1,4})[xX](?<epnumber>[0-9]{1,3})((-| - )[0-9]{1,4}[xX][eE](?<endingepnumber>[0-9]{1,3}))+[^\\\/]*$`,
		`.*(\\|\/)[sS]?(?<seasonnumber>[0-9]{1,4})[xX](?<epnumber>[0-9]{1,3})((-| - )?[xXeE](?<endingepnumber>[0-9]{1,3}))+[^\\\/]*$`,
		`.*(\\|\/)[sS]?(?<seasonnumber>[0-9]{1,4})[xX](?<epnumber>[0-9]{1,3})(-[xE]?[eE]?(?<endingepnumber>[0-9]{1,3}))+[^\\\/]*$`,
		`.*(\\|\/)(?<seriesname>((?![sS]?[0-9]{1,4}[xX][0-9]{1,3})[^\\\/])*)?([sS]?(?<seasonnumber>[0-9]{1,4})[xX](?<epnumber>[0-9]{1,3}))((-| - )[0-9]{1,4}[xXeE](?<endingepnumber>[0-9]{1,3}))+[^\\\/]*$`,
		`.*(\\|\/)(?<seriesname>((?![sS]?[0-9]{1,4}[xX][0-9]{1,3})[^\\\/])*)?([sS]?(?<seasonnumber>[0-9]{1,4})[xX](?<epnumber>[0-9]{1,3}))((-| - )[0-9]{1,4}[xX][eE](?<endingepnumber>[0-9]{1,3}))+[^\\\/]*$`,
		`.*(\\|\/)(?<seriesname>((?![sS]?[0-9]{1,4}[xX][0-9]{1,3})[^\\\/])*)?([sS]?(?<seasonnumber>[0-9]{1,4})[xX](?<epnumber>[0-9]{1,3}))((-| - )?[xXeE](?<endingepnumber>[0-9]{1,3}))+[^\\\/]*$`,
		`.*(\\|\/)(?<seriesname>((?![sS]?[0-9]{1,4}[xX][0-9]{1,3})[^\\\/])*)?([sS]?(?<seasonnumber>[0-9]{1,4})[xX](?<epnumber>[0-9]{1,3}))(-[xX]?[eE]?(?<endingepnumber>[0-9]{1,3}))+[^\\\/]*$`,
		`.*(\\|\/)(?<seriesname>[^\\\/]*)[sS](?<seasonnumber>[0-9]{1,4})[xX\.]?[eE](?<epnumber>[0-9]{1,3})((-| - )?[xXeE](?<endingepnumber>[0-9]{1,3}))+[^\\\/]*$`,
		`.*(\\|\/)(?<seriesname>[^\\\/]*)[sS](?<seasonnumber>[0-9]{1,4})[xX\.]?[eE](?<epnumber>[0-9]{1,3})(-[xX]?[eE]?(?<endingepnumber>[0-9]{1,3}))+[^\\\/]*$`,
	}
	out := make([]*EpisodeExpression, len(exps))
	for i, e := range exps {
		out[i] = &EpisodeExpression{Expression: e, IsNamed: true}
	}
	return out
}()

func compileAll(exps ...string) []*regexp2.Regexp {
	out := make([]*regexp2.Regexp, len(exps))
	for i, e := range exps {
		out[i] = regexp2.MustCompile(e, regexp2.IgnoreCase)
	}
	return out
}

func ext(path string) string { return strings.ToLower(filepath.Ext(path)) }

// IsVideoFile reports whether the path has a known video extension.
func IsVideoFile(path string) bool { _, ok := videoExtensions[ext(path)]; return ok }

// IsAudioFile reports whether the path has a known audio extension.
func IsAudioFile(path string) bool { _, ok := audioExtensions[ext(path)]; return ok }

// IsSubtitleFile reports whether the path has a known subtitle extension.
func IsSubtitleFile(path string) bool { _, ok := subtitleExtensions[ext(path)]; return ok }

// IsStubFile reports whether the path is a .disc stub.
func IsStubFile(path string) bool { _, ok := stubExtensions[ext(path)]; return ok }

func fileNameWithoutExt(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// group returns the value of a named or numbered group, or "" when absent.
func groupByName(m *regexp2.Match, name string) (string, bool) {
	g := m.GroupByName(name)
	if g == nil || len(g.Captures) == 0 {
		return "", false
	}
	return g.String(), true
}

func groupByNum(m *regexp2.Match, n int) (string, bool) {
	g := m.GroupByNumber(n)
	if g == nil || len(g.Captures) == 0 {
		return "", false
	}
	return g.String(), true
}

func match(re *regexp2.Regexp, s string) *regexp2.Match {
	m, err := re.FindStringMatch(s)
	if err != nil {
		return nil
	}
	return m
}

// atoi32 behaves like .NET int.TryParse (32-bit range).
func atoi32(s string) (int, error) {
	n, err := strconv.ParseInt(s, 10, 32)
	return int(n), err
}
