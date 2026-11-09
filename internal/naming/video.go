package naming

import (
	"path/filepath"
	"strings"
)

// VideoFileInfo mirrors Emby.Naming.Video.VideoFileInfo.
type VideoFileInfo struct {
	Path        string
	Container   string
	Name        string
	Year        int
	IsStub      bool
	StubType    string
	Is3D        bool
	Format3D    string
	ExtraType   ExtraType
	IsDirectory bool
}

// CleanDateTime extracts a trailing year from a name ("Movie (2013)" -> "Movie", 2013).
func CleanDateTime(name string) (string, int) {
	if name == "" {
		return name, 0
	}
	for _, re := range cleanDateTimeRegexes {
		m := match(re, name)
		if m == nil || m.GroupCount() != 5 {
			continue
		}
		g1, ok1 := groupByNum(m, 1)
		g2, ok2 := groupByNum(m, 2)
		if !ok1 || !ok2 {
			continue
		}
		year, err := atoi32(g2)
		if err != nil {
			continue
		}
		return strings.TrimRight(g1, " \t"), year
	}
	return name, 0
}

// CleanString iteratively strips release tags from a name. It returns the
// cleaned name and whether anything was changed.
func CleanString(name string) (string, bool) {
	if name == "" {
		return "", false
	}
	cleaned := false
	for _, re := range cleanStringRegexes {
		m := match(re, name)
		if m == nil {
			continue
		}
		if v, ok := groupByName(m, "cleaned"); ok {
			name = strings.TrimSpace(v)
			cleaned = true
		}
	}
	if !cleaned {
		return "", false
	}
	return name, true
}

// ResolveStub returns the stub type for .disc files.
func ResolveStub(path string) (string, bool) {
	if !IsStubFile(path) {
		return "", false
	}
	token := strings.TrimPrefix(filepath.Ext(fileNameWithoutExt(path)), ".")
	for _, r := range stubTypes {
		if strings.EqualFold(token, r.Token) {
			return r.Type, true
		}
	}
	return "", true
}

// Parse3D detects 3D format flags in a path.
func Parse3D(path string) (bool, string) {
	delims := videoFlagDelimiters + " "
	for _, rule := range format3DRules {
		foundPrefix := rule.Preceding == ""
		rest := path
		for len(rest) > 0 {
			idx := strings.IndexAny(rest, delims)
			var cur string
			if idx == -1 {
				cur, rest = rest, ""
			} else {
				cur, rest = rest[:idx], rest[idx+1:]
			}
			if !foundPrefix {
				foundPrefix = strings.EqualFold(cur, rule.Preceding)
				continue
			}
			if strings.EqualFold(cur, rule.Token) {
				return true, rule.Token
			}
		}
	}
	return false, ""
}

// GetExtraType determines whether a path is an extra (trailer, sample...).
func GetExtraType(path, libraryRoot string) ExtraType {
	isAudio := IsAudioFile(path)
	isVideo := IsVideoFile(path)
	trimmed := strings.TrimRight(fileNameWithoutExt(path), "0123456789")
	fullDir := filepath.Dir(path)
	dirName := filepath.Base(fullDir)
	for _, r := range videoExtraRules {
		if (r.MediaKind == kindAudio && !isAudio) || (r.MediaKind == kindVideo && !isVideo) {
			continue
		}
		var ok bool
		switch r.RuleType {
		case ruleFilename:
			ok = strings.EqualFold(trimmed, r.Token)
		case ruleSuffix:
			ok = len(trimmed) >= len(r.Token) && strings.EqualFold(trimmed[len(trimmed)-len(r.Token):], r.Token)
		case ruleDirectoryName:
			ok = strings.EqualFold(dirName, r.Token) && !strings.EqualFold(fullDir, libraryRoot)
		}
		if ok {
			return r.Type
		}
	}
	return ExtraNone
}

// ResolveVideo mirrors VideoResolver.Resolve.
func ResolveVideo(path string, isDirectory bool, parseName bool, libraryRoot string) *VideoFileInfo {
	if path == "" {
		return nil
	}
	info := &VideoFileInfo{Path: path, IsDirectory: isDirectory}
	if !isDirectory {
		if !IsVideoFile(path) {
			st, ok := ResolveStub(path)
			if !ok {
				return nil
			}
			info.IsStub, info.StubType = true, st
		}
		info.Container = strings.TrimPrefix(ext(path), ".")
	}
	info.Is3D, info.Format3D = Parse3D(path)
	info.ExtraType = GetExtraType(path, libraryRoot)
	name := filepath.Base(path)
	if !isDirectory {
		name = fileNameWithoutExt(path)
	}
	if parseName {
		name, info.Year = CleanDateTime(name)
		if n, ok := CleanString(name); ok {
			name = n
		}
	}
	info.Name = name
	return info
}
