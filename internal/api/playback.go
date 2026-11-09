package api

import (
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rubenpari/lumen/internal/db"
	"github.com/rubenpari/lumen/internal/events"
	"github.com/rubenpari/lumen/internal/stream"
)

func (s *Server) setPlayed(played bool) handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		u := userFrom(r)
		it, err := s.DB.GetItem(r.PathValue("id"))
		if err != nil {
			return err
		}
		targets := []*db.Item{it}
		switch it.Type {
		case db.TypeSeries:
			targets, err = s.DB.QueryItems(`WHERE series_id=? AND type='Episode'`, it.ID)
		case db.TypeSeason:
			targets, err = s.DB.QueryItems(`WHERE season_id=? AND type='Episode'`, it.ID)
		}
		if err != nil {
			return err
		}
		ids := make([]string, len(targets))
		for i, t := range targets {
			ids[i] = t.ID
		}
		uds, err := s.DB.GetUserData(u.ID, ids)
		if err != nil {
			return err
		}
		now := db.Now()
		for i, t := range targets {
			d := uds[t.ID]
			d.Played = played
			d.Position = 0
			if played {
				d.PlayCount++
				// Keep relative order so "next up" works after marking a season.
				d.LastPlayed = now - int64(len(targets)-i)
			}
			if err := s.DB.SaveUserData(u.ID, t.ID, d); err != nil {
				return err
			}
		}
		s.Events.Publish(events.Event{Type: "userdata.changed", User: u.ID, Data: it.ID})
		return ok2(s, w, r, it)
	}
}

func (s *Server) setFavorite(fav bool) handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		u := userFrom(r)
		id := r.PathValue("id")
		it, err := s.DB.GetItem(id)
		if err != nil {
			return err
		}
		uds, err := s.DB.GetUserData(u.ID, []string{id})
		if err != nil {
			return err
		}
		d := uds[id]
		d.Favorite = fav
		if err := s.DB.SaveUserData(u.ID, id, d); err != nil {
			return err
		}
		return ok2(s, w, r, it)
	}
}

func (s *Server) progress(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Position float64 `json:"position"`
		Duration float64 `json:"duration"`
		Paused   bool    `json:"paused"`
		Event    string  `json:"event"` // start, progress, stop
		Method   string  `json:"method"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	u := userFrom(r)
	it, err := s.DB.GetItem(r.PathValue("id"))
	if err != nil {
		return err
	}
	dur := it.Duration
	if dur == 0 && body.Duration > 0 {
		dur = body.Duration
		_, _ = s.DB.Exec(`UPDATE items SET duration=? WHERE id=?`, dur, it.ID)
	}
	uds, err := s.DB.GetUserData(u.ID, []string{it.ID})
	if err != nil {
		return err
	}
	d := uds[it.ID]
	d.LastPlayed = db.Now()
	pct := 0.0
	if dur > 0 {
		pct = body.Position / dur
	}
	switch {
	case pct >= 0.9 || (dur > 0 && dur-body.Position < 30 && dur > 300 && it.Type == db.TypeEpisode):
		if !d.Played || d.Position > 0 {
			d.PlayCount++
		}
		d.Played, d.Position = true, 0
	case pct < 0.05 && body.Position < 60:
		d.Position = 0
	default:
		d.Position = body.Position
		d.Played = false
	}
	if err := s.DB.SaveUserData(u.ID, it.ID, d); err != nil {
		return err
	}
	if body.Event == "stop" {
		s.playing.Delete(u.ID)
		s.Events.Publish(events.Event{Type: "userdata.changed", User: u.ID, Data: it.ID})
	} else {
		np := &NowPlaying{UserID: u.ID, UserName: u.Name, ItemID: it.ID, ItemName: it.Name, Position: body.Position,
			Duration: dur, Paused: body.Paused, Method: body.Method, Device: deviceName(r.UserAgent()), UpdatedAt: db.Now()}
		if it.SeriesID != "" {
			if sr, err := s.DB.GetItem(it.SeriesID); err == nil {
				np.Series = sr.Name
			}
		}
		s.playing.Store(u.ID, np)
	}
	return ok(w, d)
}

func deviceName(ua string) string {
	browser := "Browser"
	switch {
	case strings.Contains(ua, "Edg/"):
		browser = "Edge"
	case strings.Contains(ua, "Firefox/"):
		browser = "Firefox"
	case strings.Contains(ua, "Chrome/"):
		browser = "Chrome"
	case strings.Contains(ua, "Safari/"):
		browser = "Safari"
	}
	osName := ""
	switch {
	case strings.Contains(ua, "Windows"):
		osName = "Windows"
	case strings.Contains(ua, "Android"):
		osName = "Android"
	case strings.Contains(ua, "iPhone"), strings.Contains(ua, "iPad"):
		osName = "iOS"
	case strings.Contains(ua, "Mac OS"):
		osName = "macOS"
	case strings.Contains(ua, "Linux"):
		osName = "Linux"
	}
	if osName == "" {
		return browser
	}
	return browser + " on " + osName
}

type subtitleTrack struct {
	Index    int    `json:"index"` // position used in the subtitles URL
	Label    string `json:"label"`
	Language string `json:"language,omitempty"`
	Codec    string `json:"codec"`
	External bool   `json:"external"`
	Forced   bool   `json:"forced,omitempty"`
	Default  bool   `json:"default,omitempty"`
	URL      string `json:"url,omitempty"`    // empty when not convertible (image subs)
	BurnIn   bool   `json:"burnIn,omitempty"` // image subtitle: must be burned in while transcoding
}

type audioTrack struct {
	Index    int    `json:"index"`
	Label    string `json:"label"`
	Language string `json:"language,omitempty"`
	Codec    string `json:"codec"`
	Channels int    `json:"channels"`
	Default  bool   `json:"default,omitempty"`
}

var langLabels = map[string]string{
	"eng": "English", "en": "English", "ita": "Italiano", "it": "Italiano", "fre": "Français", "fra": "Français", "fr": "Français",
	"ger": "Deutsch", "deu": "Deutsch", "de": "Deutsch", "spa": "Español", "es": "Español", "por": "Português", "pt": "Português",
	"jpn": "日本語", "ja": "日本語", "rus": "Русский", "ru": "Русский", "chi": "中文", "zho": "中文", "kor": "한국어", "dut": "Nederlands",
	"nld": "Nederlands", "swe": "Svenska", "pol": "Polski", "tur": "Türkçe", "ara": "العربية", "und": "Unknown",
}

func langLabel(code string) string {
	if l, ok := langLabels[strings.ToLower(code)]; ok {
		return l
	}
	if code == "" {
		return "Unknown"
	}
	return strings.ToUpper(code)
}

func channelLabel(c int) string {
	switch c {
	case 1:
		return "Mono"
	case 2:
		return "Stereo"
	case 6:
		return "5.1"
	case 8:
		return "7.1"
	}
	if c == 0 {
		return ""
	}
	return fmt.Sprintf("%dch", c)
}

// subtitleTracks lists embedded then external subtitles. Index i maps to:
// i < len(embedded): embedded stream; otherwise external file i-len(embedded).
func subtitleTracks(it *db.Item) []subtitleTrack {
	var out []subtitleTrack
	i := 0
	for _, st := range it.Media.Streams {
		if st.Type != "subtitle" {
			continue
		}
		label := langLabel(st.Language)
		if st.Title != "" {
			label += " · " + st.Title
		}
		t := subtitleTrack{Index: i, Label: label, Language: st.Language, Codec: st.Codec, Forced: st.Forced, Default: st.Default}
		if stream.IsTextSubtitleCodec(st.Codec) {
			t.URL = fmt.Sprintf("/api/items/%s/subtitles/%d", it.ID, i)
		} else if stream.IsImageSubtitleCodec(st.Codec) {
			t.BurnIn = true
		}
		out = append(out, t)
		i++
	}
	for _, ex := range it.Subtitles {
		label := langLabel(ex.Language)
		if ex.Forced {
			label += " · Forced"
		}
		if ex.SDH {
			label += " · SDH"
		}
		label += " (" + strings.ToUpper(ex.Format) + ")"
		t := subtitleTrack{Index: i, Label: label, Language: ex.Language, Codec: ex.Format, External: true, Forced: ex.Forced, Default: ex.Default}
		switch ex.Format {
		case "srt", "vtt", "ass", "ssa":
			t.URL = fmt.Sprintf("/api/items/%s/subtitles/%d", it.ID, i)
		}
		out = append(out, t)
		i++
	}
	return out
}

func (s *Server) playbackInfo(w http.ResponseWriter, r *http.Request) error {
	it, err := s.DB.GetItem(r.PathValue("id"))
	if err != nil {
		return err
	}
	if it.IsFolder {
		return errf(http.StatusBadRequest, "item is not playable")
	}
	var audio []audioTrack
	var videoCodec, audioCodec string
	var width, height int
	var hdr bool
	for _, st := range it.Media.Streams {
		switch st.Type {
		case "audio":
			label := langLabel(st.Language)
			if c := channelLabel(st.Channels); c != "" {
				label += " · " + c
			}
			label += " · " + strings.ToUpper(st.Codec)
			if st.Title != "" {
				label = st.Title + " (" + label + ")"
			}
			audio = append(audio, audioTrack{st.Index, label, st.Language, st.Codec, st.Channels, st.Default})
			if audioCodec == "" {
				audioCodec = st.Codec
			}
		case "video":
			if videoCodec == "" {
				videoCodec, width, height, hdr = st.Codec, st.Width, st.Height, st.HDR
			}
		}
	}
	d, err := s.one(r, it)
	if err != nil {
		return err
	}
	if audio == nil {
		audio = []audioTrack{}
	}
	subs := subtitleTracks(it)
	if subs == nil {
		subs = []subtitleTrack{}
	}
	return ok(w, map[string]any{
		"item":         d,
		"directUrl":    "/api/items/" + it.ID + "/stream",
		"container":    it.Container,
		"mimeType":     mime.TypeByExtension(filepath.Ext(it.Path)),
		"videoCodec":   videoCodec,
		"audioCodec":   audioCodec,
		"width":        width,
		"height":       height,
		"hdr":          hdr,
		"probed":       it.Media.Probed,
		"canTranscode": s.FFmpeg != "",
		"audioTracks":  audio,
		"subtitles":    subs,
		"chapters":     it.Media.Chapters,
		"duration":     it.Duration,
	})
}

func (s *Server) directStream(w http.ResponseWriter, r *http.Request) error {
	it, err := s.DB.GetItem(r.PathValue("id"))
	if err != nil {
		return err
	}
	if it.IsFolder || it.DiscType != "" {
		return errf(http.StatusBadRequest, "not directly playable")
	}
	if ct := mime.TypeByExtension(filepath.Ext(it.Path)); ct != "" {
		w.Header().Set("Content-Type", ct)
	} else if strings.EqualFold(filepath.Ext(it.Path), ".mkv") {
		w.Header().Set("Content-Type", "video/x-matroska")
	}
	http.ServeFile(w, r, it.Path)
	return nil
}

func (s *Server) startHLS(w http.ResponseWriter, r *http.Request) error {
	if s.FFmpeg == "" {
		return errf(http.StatusNotImplemented, "transcoding requires ffmpeg on the server")
	}
	it, err := s.DB.GetItem(r.PathValue("id"))
	if err != nil {
		return err
	}
	var body struct {
		Start      float64 `json:"start"`
		Audio      *int    `json:"audio"`
		MaxHeight  int     `json:"maxHeight"`
		MaxBitrate int     `json:"maxBitrate"`
		ForceVideo bool    `json:"forceVideo"`
		BurnSub    *int    `json:"burnSubtitle"` // subtitle track index (as in playback info)
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	opt := stream.Options{Start: body.Start, AudioStream: -1, MaxHeight: body.MaxHeight, MaxBitrate: body.MaxBitrate, ForceVideo: body.ForceVideo}
	if body.Audio != nil {
		opt.AudioStream = *body.Audio
	}
	opt.BurnSubtitle = -1
	if body.BurnSub != nil {
		n := 0
		for _, st := range it.Media.Streams {
			if st.Type != "subtitle" {
				continue
			}
			if n == *body.BurnSub {
				opt.BurnSubtitle = st.Index
			}
			n++
		}
	}
	sess, err := s.Streams.Start(it, opt, userFrom(r).ID)
	if err != nil {
		return err
	}
	if err := sess.WaitPlaylist(r.Context()); err != nil {
		s.Streams.Stop(sess.ID)
		return err
	}
	return ok(w, map[string]any{"sessionId": sess.ID, "url": "/api/hls/" + sess.ID + "/index.m3u8", "start": sess.Start})
}

func (s *Server) hlsFile(w http.ResponseWriter, r *http.Request) error {
	sid := r.PathValue("sid")
	if !strings.HasPrefix(sid, userFrom(r).ID) {
		return errf(http.StatusForbidden, "not your session")
	}
	sess, found := s.Streams.Get(sid)
	if !found {
		return errf(http.StatusNotFound, "session expired")
	}
	file := filepath.Base(r.PathValue("file"))
	switch {
	case file == "index.m3u8":
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, filepath.Join(sess.Dir, file))
	case strings.HasSuffix(file, ".ts"):
		p, err := sess.WaitSegment(r.Context(), file)
		if err != nil {
			return errf(http.StatusNotFound, "segment not available")
		}
		w.Header().Set("Content-Type", "video/mp2t")
		http.ServeFile(w, r, p)
	default:
		return errf(http.StatusNotFound, "not found")
	}
	return nil
}

func (s *Server) stopHLS(w http.ResponseWriter, r *http.Request) error {
	sid := r.PathValue("sid")
	if strings.HasPrefix(sid, userFrom(r).ID) {
		s.Streams.Stop(sid)
	}
	return ok(w, map[string]bool{"ok": true})
}

func (s *Server) subtitle(w http.ResponseWriter, r *http.Request) error {
	it, err := s.DB.GetItem(r.PathValue("id"))
	if err != nil {
		return err
	}
	idx, err := strconv.Atoi(strings.TrimSuffix(r.PathValue("idx"), ".vtt"))
	if err != nil {
		return errf(http.StatusBadRequest, "bad index")
	}
	var embedded []db.Stream
	for _, st := range it.Media.Streams {
		if st.Type == "subtitle" {
			embedded = append(embedded, st)
		}
	}
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	if idx < len(embedded) {
		if s.FFmpeg == "" {
			return errf(http.StatusNotImplemented, "extracting embedded subtitles requires ffmpeg")
		}
		return stream.ExtractEmbedded(r.Context(), s.FFmpeg, it.Path, embedded[idx].Index, w)
	}
	idx -= len(embedded)
	if idx < 0 || idx >= len(it.Subtitles) {
		return errf(http.StatusNotFound, "no such subtitle")
	}
	ex := it.Subtitles[idx]
	vtt, err := stream.ExternalToVTT(ex.Path, ex.Format)
	if err != nil {
		return errf(http.StatusUnsupportedMediaType, err.Error())
	}
	_, err = w.Write([]byte(vtt))
	return err
}
