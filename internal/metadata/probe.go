package metadata

import (
	"context"
	"encoding/json"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/rubenpari/lumen/internal/db"
)

// Probe runs ffprobe on a file and returns technical media information.
func Probe(ctx context.Context, ffprobe, path string) (db.MediaInfo, float64, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, ffprobe, "-v", "quiet", "-print_format", "json",
		"-show_format", "-show_streams", "-show_chapters", path).Output()
	if err != nil {
		return db.MediaInfo{}, 0, err
	}
	var res struct {
		Format struct {
			FormatName string `json:"format_name"`
			Duration   string `json:"duration"`
			BitRate    string `json:"bit_rate"`
		} `json:"format"`
		Streams []struct {
			Index        int    `json:"index"`
			CodecType    string `json:"codec_type"`
			CodecName    string `json:"codec_name"`
			Profile      string `json:"profile"`
			Channels     int    `json:"channels"`
			Width        int    `json:"width"`
			Height       int    `json:"height"`
			BitRate      string `json:"bit_rate"`
			AvgFrameRate string `json:"avg_frame_rate"`
			PixFmt       string `json:"pix_fmt"`
			ColorTrans   string `json:"color_transfer"`
			Disposition  struct {
				Default  int `json:"default"`
				Forced   int `json:"forced"`
				Attached int `json:"attached_pic"`
			} `json:"disposition"`
			Tags map[string]string `json:"tags"`
		} `json:"streams"`
		Chapters []struct {
			StartTime string            `json:"start_time"`
			Tags      map[string]string `json:"tags"`
		} `json:"chapters"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return db.MediaInfo{}, 0, err
	}
	mi := db.MediaInfo{Container: res.Format.FormatName, Probed: true}
	mi.BitRate, _ = strconv.ParseInt(res.Format.BitRate, 10, 64)
	dur, _ := strconv.ParseFloat(res.Format.Duration, 64)
	for _, s := range res.Streams {
		if s.Disposition.Attached == 1 {
			continue
		}
		st := db.Stream{
			Index: s.Index, Type: s.CodecType, Codec: s.CodecName, Profile: s.Profile, Channels: s.Channels,
			Width: s.Width, Height: s.Height, PixFmt: s.PixFmt,
			Default: s.Disposition.Default == 1, Forced: s.Disposition.Forced == 1,
			HDR: s.ColorTrans == "smpte2084" || s.ColorTrans == "arib-std-b67",
		}
		st.BitRate, _ = strconv.ParseInt(s.BitRate, 10, 64)
		if p := strings.Split(s.AvgFrameRate, "/"); len(p) == 2 {
			a, _ := strconv.ParseFloat(p[0], 64)
			b, _ := strconv.ParseFloat(p[1], 64)
			if b > 0 {
				st.FrameRate = a / b
			}
		}
		for k, v := range s.Tags {
			switch strings.ToLower(k) {
			case "language":
				st.Language = v
			case "title":
				st.Title = v
			}
		}
		switch st.Type {
		case "video", "audio", "subtitle":
			mi.Streams = append(mi.Streams, st)
		}
	}
	for _, c := range res.Chapters {
		start, _ := strconv.ParseFloat(c.StartTime, 64)
		mi.Chapters = append(mi.Chapters, struct {
			Start float64 `json:"start"`
			Title string  `json:"title"`
		}{start, c.Tags["title"]})
	}
	return mi, dur, nil
}
