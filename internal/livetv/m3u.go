// Package livetv implements Live TV from M3U playlists and XMLTV guides
// (port of Jellyfin.LiveTv TunerHosts/M3uParser and Listings/XmlTv).
package livetv

import (
	"bufio"
	"io"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Channel is a channel parsed from an M3U playlist.
type Channel struct {
	Number    string
	Name      string
	Logo      string
	Group     string
	TvgID     string
	URL       string
	UserAgent string
}

var kvRe = regexp.MustCompile(`(?i)([a-z0-9\-_]+)="([^"]+)"`)

func parseExtInf(line string) (map[string]string, string) {
	attrs := map[string]string{}
	remaining := line
	for _, m := range kvRe.FindAllStringSubmatch(line, -1) {
		attrs[strings.ToLower(m[1])] = m[2]
		remaining = strings.Replace(remaining, m[0], "", 1)
	}
	return attrs, remaining
}

// validURL accepts the schemes Jellyfin accepts plus local files (useful for testing).
func validURL(u string) bool {
	if strings.HasPrefix(u, "/") {
		return true
	}
	p, err := url.Parse(u)
	if err != nil {
		return false
	}
	switch strings.ToLower(p.Scheme) {
	case "http", "https", "rtsp", "rtp", "udp", "rtmp", "file":
		return true
	}
	return false
}

func channelName(extInf string, attrs map[string]string) (string, string) {
	parts := strings.Split(extInf, ",")
	name := ""
	number := ""
	if len(parts) > 1 {
		name = strings.TrimSpace(parts[len(parts)-1])
	}
	// "84. VOX Schweiz" / "84.0 - VOX Schweiz" (SAT>IP style)
	if i := strings.IndexByte(name, ' '); i > 0 {
		numPart := strings.Trim(name[:i], " .")
		if _, err := strconv.ParseFloat(numPart, 64); err == nil {
			number = numPart
			name = strings.Trim(name[i+1:], " -")
		}
	}
	if name == "" {
		name = attrs["tvg-name"]
	}
	if name == "" {
		name = attrs["tvg-id"]
	}
	return name, number
}

// ParseM3U parses an M3U playlist.
func ParseM3U(r io.Reader) []Channel {
	var out []Channel
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	extInf := ""
	ua := ""
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\ufeff"))
		switch {
		case line == "", strings.HasPrefix(strings.ToUpper(line), "#EXTM3U"):
		case strings.HasPrefix(strings.ToUpper(line), "#EXTINF:"):
			extInf = strings.TrimSpace(line[len("#EXTINF:"):])
			ua = ""
		case strings.HasPrefix(strings.ToLower(line), "#extvlcopt:http-user-agent="):
			ua = line[len("#EXTVLCOPT:http-user-agent="):]
		case strings.HasPrefix(line, "#"):
		case extInf != "":
			if !validURL(line) {
				extInf = ""
				continue
			}
			attrs, rest := parseExtInf(extInf)
			name, number := channelName(rest, attrs)
			if n := firstNonEmpty(attrs["tvg-chno"], attrs["channel-number"], attrs["tvg-num"]); n != "" {
				number = n
			}
			if name == "" {
				name = strings.TrimSuffix(filepath.Base(line), filepath.Ext(line))
			}
			ch := Channel{
				Number: number, Name: name, URL: line, UserAgent: ua,
				Logo: firstNonEmpty(attrs["tvg-logo"], attrs["logo"]), Group: attrs["group-title"],
				TvgID: firstNonEmpty(attrs["tvg-id"], attrs["channel-id"]),
			}
			out = append(out, ch)
			extInf = ""
		}
	}
	for i := range out {
		if out[i].Number == "" {
			out[i].Number = strconv.Itoa(i + 1)
		}
	}
	return out
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}
