// Package stream handles direct play, transcoding and subtitle conversion.
package stream

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

// decodeText converts Latin-1 / Windows-1252 subtitles to UTF-8 when needed.
func decodeText(b []byte) string {
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	if utf8.Valid(b) {
		return string(b)
	}
	out, err := charmap.Windows1252.NewDecoder().Bytes(b)
	if err != nil {
		return string(b)
	}
	return string(out)
}

var srtTime = regexp.MustCompile(`(\d{1,2}):(\d{2}):(\d{2})[,.](\d{1,3})`)

// SRTToVTT converts SubRip text to WebVTT.
func SRTToVTT(src string) string {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")
	var b strings.Builder
	b.WriteString("WEBVTT\n\n")
	for _, block := range strings.Split(strings.TrimSpace(src), "\n\n") {
		lines := strings.Split(strings.TrimSpace(block), "\n")
		for i, l := range lines {
			if strings.Contains(l, "-->") {
				l = srtTime.ReplaceAllStringFunc(l, func(t string) string {
					m := srtTime.FindStringSubmatch(t)
					ms := m[4]
					for len(ms) < 3 {
						ms += "0"
					}
					h, _ := strconv.Atoi(m[1])
					return fmt.Sprintf("%02d:%s:%s.%s", h, m[2], m[3], ms)
				})
				b.WriteString(l + "\n")
				for _, t := range lines[i+1:] {
					b.WriteString(stripASSTags(t) + "\n")
				}
				b.WriteString("\n")
				break
			}
		}
	}
	return b.String()
}

var assTag = regexp.MustCompile(`\{[^}]*\}`)

func stripASSTags(s string) string { return assTag.ReplaceAllString(s, "") }

func assTime(t string) string {
	// h:mm:ss.cc -> hh:mm:ss.mmm
	p := strings.Split(strings.TrimSpace(t), ":")
	if len(p) != 3 {
		return "00:00:00.000"
	}
	h, _ := strconv.Atoi(p[0])
	sec := strings.SplitN(p[2], ".", 2)
	cs := "000"
	if len(sec) == 2 {
		cs = (sec[1] + "000")[:3]
		if len(sec[1]) == 2 {
			cs = sec[1] + "0"
		}
	}
	return fmt.Sprintf("%02d:%s:%s.%s", h, p[1], sec[0], cs)
}

// ASSToVTT converts SubStation Alpha subtitles to WebVTT (styling is dropped).
func ASSToVTT(src string) string {
	var b strings.Builder
	b.WriteString("WEBVTT\n\n")
	sc := bufio.NewScanner(strings.NewReader(src))
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	format := []string{"layer", "start", "end", "style", "name", "marginl", "marginr", "marginv", "effect", "text"}
	inEvents := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			inEvents = strings.EqualFold(line, "[Events]")
			continue
		}
		if !inEvents {
			continue
		}
		if strings.HasPrefix(line, "Format:") {
			format = nil
			for _, f := range strings.Split(strings.TrimPrefix(line, "Format:"), ",") {
				format = append(format, strings.ToLower(strings.TrimSpace(f)))
			}
			continue
		}
		if !strings.HasPrefix(line, "Dialogue:") {
			continue
		}
		fields := strings.SplitN(strings.TrimPrefix(line, "Dialogue:"), ",", len(format))
		if len(fields) < len(format) {
			continue
		}
		var start, end, text string
		for i, f := range format {
			switch f {
			case "start":
				start = fields[i]
			case "end":
				end = fields[i]
			case "text":
				text = fields[i]
			}
		}
		text = stripASSTags(text)
		text = strings.ReplaceAll(strings.ReplaceAll(text, `\N`, "\n"), `\n`, "\n")
		text = strings.ReplaceAll(text, `\h`, " ")
		if strings.TrimSpace(text) == "" {
			continue
		}
		fmt.Fprintf(&b, "%s --> %s\n%s\n\n", assTime(start), assTime(end), text)
	}
	return b.String()
}

// ExternalToVTT converts a subtitle file on disk to WebVTT.
func ExternalToVTT(path, format string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	text := decodeText(data)
	switch format {
	case "vtt":
		return text, nil
	case "srt":
		return SRTToVTT(text), nil
	case "ass", "ssa":
		return ASSToVTT(text), nil
	}
	return "", fmt.Errorf("unsupported subtitle format %q", format)
}

// ExtractEmbedded extracts an embedded text subtitle stream as WebVTT via ffmpeg.
func ExtractEmbedded(ctx context.Context, ffmpeg, path string, streamIndex int, w io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffmpeg, "-v", "error", "-i", path, "-map", fmt.Sprintf("0:%d", streamIndex), "-f", "webvtt", "pipe:1")
	cmd.Stdout = w
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg: %w: %s", err, stderr.String())
	}
	return nil
}

// IsTextSubtitleCodec reports whether an embedded codec can be converted to WebVTT.
func IsTextSubtitleCodec(codec string) bool {
	switch codec {
	case "subrip", "srt", "ass", "ssa", "webvtt", "mov_text", "text":
		return true
	}
	return false
}

// IsImageSubtitleCodec reports whether a subtitle codec is bitmap based and
// therefore has to be burned into the video.
func IsImageSubtitleCodec(codec string) bool {
	switch codec {
	case "hdmv_pgs_subtitle", "pgssub", "dvd_subtitle", "dvdsub", "dvb_subtitle", "xsub":
		return true
	}
	return false
}
