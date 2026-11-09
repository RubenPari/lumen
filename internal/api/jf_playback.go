package api

import (
	"encoding/json"
	"fmt"
	"math"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rubenpari/lumen/internal/db"
	"github.com/rubenpari/lumen/internal/stream"
)

const jfSegment = 6 // seconds per HLS segment for Jellyfin clients

func codecDisplay(c string) string {
	switch c {
	case "h264":
		return "H264"
	case "hevc":
		return "HEVC"
	case "aac":
		return "AAC"
	case "ac3":
		return "Dolby Digital"
	case "eac3":
		return "Dolby Digital+"
	case "dts":
		return "DTS"
	case "truehd":
		return "TrueHD"
	case "subrip":
		return "SUBRIP"
	}
	return strings.ToUpper(c)
}

// jfStreams builds MediaStreams: embedded streams keep their ffprobe index,
// external subtitles get indexes after the last embedded stream.
func (s *Server) jfStreams(it *db.Item) []map[string]any {
	out := []map[string]any{}
	maxIdx := -1
	for _, st := range it.Media.Streams {
		if st.Index > maxIdx {
			maxIdx = st.Index
		}
		m := map[string]any{
			"Codec": st.Codec, "Language": st.Language, "Title": st.Title, "IsDefault": st.Default, "IsForced": st.Forced,
			"Index": st.Index, "IsExternal": false, "IsInterlaced": false, "Profile": st.Profile, "IsHearingImpaired": false,
		}
		lang := langLabel(st.Language)
		switch st.Type {
		case "video":
			m["Type"] = "Video"
			m["Width"], m["Height"] = st.Width, st.Height
			m["PixelFormat"] = st.PixFmt
			if st.FrameRate > 0 {
				m["AverageFrameRate"], m["RealFrameRate"] = st.FrameRate, st.FrameRate
			}
			if st.BitRate > 0 {
				m["BitRate"] = st.BitRate
			}
			rng := "SDR"
			if st.HDR {
				rng = "HDR"
			}
			m["VideoRange"], m["VideoRangeType"] = rng, rng
			if strings.Contains(st.PixFmt, "10") {
				m["BitDepth"] = 10
			} else {
				m["BitDepth"] = 8
			}
			m["DisplayTitle"] = strings.TrimSpace(fmt.Sprintf("%s %s %s", resolutionName(st.Width, st.Height), codecDisplay(st.Codec), rng))
		case "audio":
			m["Type"] = "Audio"
			m["Channels"] = st.Channels
			m["ChannelLayout"] = channelLabel(st.Channels)
			if st.BitRate > 0 {
				m["BitRate"] = st.BitRate
			}
			m["DisplayTitle"] = strings.TrimSpace(fmt.Sprintf("%s - %s - %s", lang, codecDisplay(st.Codec), channelLabel(st.Channels)))
		case "subtitle":
			m["Type"] = "Subtitle"
			text := stream.IsTextSubtitleCodec(st.Codec)
			m["IsTextSubtitleStream"] = text
			m["SupportsExternalStream"] = text
			if text {
				m["DeliveryMethod"] = "External"
				m["DeliveryUrl"] = fmt.Sprintf("/Videos/%s/%s/Subtitles/%d/0/Stream.vtt", it.ID, it.ID, st.Index)
			} else {
				m["DeliveryMethod"] = "Encode"
			}
			m["DisplayTitle"] = strings.TrimSpace(lang + " - " + codecDisplay(st.Codec))
		default:
			continue
		}
		out = append(out, m)
	}
	for i, ex := range it.Subtitles {
		idx := maxIdx + 1 + i
		title := langLabel(ex.Language)
		if ex.Forced {
			title += " (Forced)"
		}
		codec := ex.Format
		if codec == "srt" {
			codec = "subrip"
		}
		out = append(out, map[string]any{
			"Codec":    codec,
			"Language": ex.Language, "DisplayTitle": title + " - " + strings.ToUpper(ex.Format) + " - External", "Type": "Subtitle",
			"Index": idx, "IsExternal": true, "IsForced": ex.Forced, "IsDefault": ex.Default, "IsHearingImpaired": ex.SDH,
			"IsTextSubtitleStream": true, "SupportsExternalStream": true, "DeliveryMethod": "External",
			"DeliveryUrl": fmt.Sprintf("/Videos/%s/%s/Subtitles/%d/0/Stream.vtt", it.ID, it.ID, idx),
		})
	}
	return out
}

func resolutionName(w, h int) string {
	switch {
	case w >= 3800 || h >= 2100:
		return "4K"
	case w >= 1900 || h >= 1000:
		return "1080p"
	case w >= 1200 || h >= 700:
		return "720p"
	case h > 0:
		return "SD"
	}
	return ""
}

func (s *Server) jfMediaSource(it *db.Item) map[string]any {
	streams := s.jfStreams(it)
	src := map[string]any{
		"Protocol": "File", "Id": it.ID, "Path": it.Path, "Type": "Default", "Container": it.Container, "Size": it.Size,
		"Name": strings.TrimSuffix(filepath.Base(it.Path), filepath.Ext(it.Path)), "IsRemote": false, "ETag": imgTag(fmt.Sprint(it.MTime)),
		"RunTimeTicks": toTicks(it.Duration), "ReadAtNativeFramerate": false, "IgnoreDts": false, "IgnoreIndex": false, "GenPtsInput": false,
		"SupportsTranscoding": s.FFmpeg != "", "SupportsDirectStream": it.DiscType == "", "SupportsDirectPlay": it.DiscType == "",
		"IsInfiniteStream": false, "RequiresOpening": false, "RequiresClosing": false, "RequiresLooping": false, "SupportsProbing": true,
		"MediaStreams": streams, "MediaAttachments": []any{}, "Formats": []any{}, "RequiredHttpHeaders": map[string]string{},
		"TranscodingSubProtocol": "http", "HasSegments": false,
	}
	if it.Type != db.TypeAudio {
		src["VideoType"] = "VideoFile"
	}
	if it.Media.BitRate > 0 {
		src["Bitrate"] = it.Media.BitRate
	}
	for _, st := range it.Media.Streams {
		if st.Type == "audio" {
			if _, set := src["DefaultAudioStreamIndex"]; !set || st.Default {
				src["DefaultAudioStreamIndex"] = st.Index
			}
		}
	}
	return src
}

type deviceProfile struct {
	MaxStreamingBitrate int64
	DirectPlayProfiles  []struct {
		Container  string
		AudioCodec string
		VideoCodec string
		Type       string
	}
}

func listHas(list, v string) bool {
	if strings.TrimSpace(list) == "" {
		return true // empty list means "any"
	}
	for _, x := range strings.Split(strings.ToLower(list), ",") {
		if strings.TrimSpace(x) == v {
			return true
		}
	}
	return false
}

// canDirectPlay evaluates the client's DirectPlayProfiles.
func canDirectPlay(it *db.Item, p *deviceProfile, audioIndex int) bool {
	if it.DiscType != "" {
		return false
	}
	if p == nil || len(p.DirectPlayProfiles) == 0 {
		return true
	}
	var vcodec, acodec string
	for _, st := range it.Media.Streams {
		if st.Type == "video" && vcodec == "" {
			vcodec = st.Codec
		}
		if st.Type == "audio" && (acodec == "" || st.Index == audioIndex) {
			acodec = st.Codec
		}
	}
	container := strings.ToLower(it.Container)
	aliases := map[string][]string{"mkv": {"mkv", "matroska", "webm"}, "mp4": {"mp4", "m4v", "mov"}, "m4v": {"mp4", "m4v"}, "mov": {"mov", "mp4"}}
	want := "Video"
	if it.Type == db.TypeAudio {
		want = "Audio"
	}
	for _, dp := range p.DirectPlayProfiles {
		if !strings.EqualFold(dp.Type, want) {
			continue
		}
		okContainer := false
		for _, c := range append([]string{container}, aliases[container]...) {
			if listHas(dp.Container, c) {
				okContainer = true
				break
			}
		}
		if !okContainer {
			continue
		}
		if want == "Video" && vcodec != "" && !listHas(dp.VideoCodec, vcodec) {
			continue
		}
		if acodec != "" && !listHas(dp.AudioCodec, acodec) {
			continue
		}
		return true
	}
	return false
}

func (s *Server) jfPlaybackInfo(w http.ResponseWriter, r *http.Request) error {
	it, err := s.DB.GetItem(normID(jfParam(r, "itemId")))
	if err != nil {
		return err
	}
	var body struct {
		DeviceProfile       *deviceProfile
		MaxStreamingBitrate int64
		StartTimeTicks      int64
		AudioStreamIndex    *int
		SubtitleStreamIndex *int
		EnableDirectPlay    *bool
		EnableDirectStream  *bool
		EnableTranscoding   *bool
	}
	if r.Method == http.MethodPost && r.ContentLength != 0 {
		_ = decode(r, &body)
	}
	if v, err := strconv.ParseInt(jfQuery(r, "MaxStreamingBitrate"), 10, 64); err == nil {
		body.MaxStreamingBitrate = v
	}
	if v, err := strconv.Atoi(jfQuery(r, "AudioStreamIndex")); err == nil {
		body.AudioStreamIndex = &v
	}
	if v, err := strconv.Atoi(jfQuery(r, "SubtitleStreamIndex")); err == nil {
		body.SubtitleStreamIndex = &v
	}
	if body.DeviceProfile != nil && body.MaxStreamingBitrate == 0 {
		body.MaxStreamingBitrate = body.DeviceProfile.MaxStreamingBitrate
	}
	audio := -1
	if body.AudioStreamIndex != nil {
		audio = *body.AudioStreamIndex
	}
	src := s.jfMediaSource(it)
	direct := canDirectPlay(it, body.DeviceProfile, audio)
	if body.EnableDirectPlay != nil && !*body.EnableDirectPlay {
		direct = false
	}
	if body.MaxStreamingBitrate > 0 && it.Media.BitRate > body.MaxStreamingBitrate {
		direct = false
	}
	// Burning image subtitles requires transcoding.
	burn := -1
	if body.SubtitleStreamIndex != nil && *body.SubtitleStreamIndex >= 0 {
		for _, st := range it.Media.Streams {
			if st.Type == "subtitle" && st.Index == *body.SubtitleStreamIndex && stream.IsImageSubtitleCodec(st.Codec) {
				burn = st.Index
				direct = false
			}
		}
	}
	playSession := db.NewID()
	src["SupportsDirectPlay"] = direct
	src["SupportsDirectStream"] = direct
	if !direct && s.FFmpeg != "" && it.Type != db.TypeAudio {
		q := url.Values{}
		q.Set("MediaSourceId", it.ID)
		q.Set("PlaySessionId", playSession)
		q.Set("api_key", parseJFAuth(r).Token)
		if audio >= 0 {
			q.Set("AudioStreamIndex", strconv.Itoa(audio))
		}
		if burn >= 0 {
			q.Set("SubtitleStreamIndex", strconv.Itoa(burn))
		}
		if body.MaxStreamingBitrate > 0 {
			q.Set("VideoBitrate", strconv.FormatInt(body.MaxStreamingBitrate, 10))
		}
		src["TranscodingUrl"] = "/videos/" + it.ID + "/master.m3u8?" + q.Encode()
		src["TranscodingSubProtocol"] = "hls"
		src["TranscodingContainer"] = "ts"
		src["TranscodeReasons"] = "ContainerNotSupported"
	} else if !direct && it.Type == db.TypeAudio && s.FFmpeg != "" {
		src["TranscodingUrl"] = "/Audio/" + it.ID + "/stream.mp3?static=false&api_key=" + parseJFAuth(r).Token
		src["TranscodingSubProtocol"] = "http"
		src["TranscodingContainer"] = "mp3"
	}
	return jfJSON(w, map[string]any{"MediaSources": []any{src}, "PlaySessionId": playSession})
}

// jfStream serves the original file (direct play / direct stream).
func (s *Server) jfStream(w http.ResponseWriter, r *http.Request) error {
	it, err := s.DB.GetItem(normID(jfParam(r, "itemId")))
	if err != nil {
		return err
	}
	if it.IsFolder || it.DiscType != "" {
		return errf(http.StatusBadRequest, "not directly playable")
	}
	if it.Type == db.TypeAudio && strings.EqualFold(jfQuery(r, "static"), "false") && s.FFmpeg != "" {
		return s.jfTranscodeAudio(w, r, it)
	}
	if ct := mime.TypeByExtension(strings.ToLower(filepath.Ext(it.Path))); ct != "" {
		w.Header().Set("Content-Type", ct)
	} else if strings.EqualFold(filepath.Ext(it.Path), ".mkv") {
		w.Header().Set("Content-Type", "video/x-matroska")
	}
	http.ServeFile(w, r, it.Path)
	return nil
}

func (s *Server) jfTranscodeAudio(w http.ResponseWriter, r *http.Request, it *db.Item) error {
	r2 := r.Clone(r.Context())
	r2.SetPathValue("id", it.ID)
	q := r2.URL.Query()
	if t, err := strconv.ParseInt(jfQuery(r, "StartTimeTicks"), 10, 64); err == nil {
		q.Set("start", strconv.FormatFloat(fromTicks(t), 'f', 3, 64))
	}
	r2.URL.RawQuery = q.Encode()
	return s.transcodeAudio(w, r2)
}

// jfUniversalAudio implements /Audio/{id}/universal: direct when the client
// supports the container, MP3 transcode otherwise.
func (s *Server) jfUniversalAudio(w http.ResponseWriter, r *http.Request) error {
	it, err := s.DB.GetItem(normID(jfParam(r, "itemId")))
	if err != nil {
		return err
	}
	containers := strings.ToLower(jfQuery(r, "Container"))
	if containers == "" || listHas(containers, strings.ToLower(it.Container)) || s.FFmpeg == "" {
		http.ServeFile(w, r, it.Path)
		return nil
	}
	return s.jfTranscodeAudio(w, r, it)
}

// ---- HLS (VOD playlist + on-demand segments) ----

func (s *Server) jfMasterPlaylist(w http.ResponseWriter, r *http.Request) error {
	it, err := s.DB.GetItem(normID(jfParam(r, "itemId")))
	if err != nil {
		return err
	}
	bw := int64(8_000_000)
	if v, err := strconv.ParseInt(jfQuery(r, "VideoBitrate"), 10, 64); err == nil && v > 0 {
		bw = v
	}
	res := ""
	for _, st := range it.Media.Streams {
		if st.Type == "video" && st.Width > 0 {
			h := st.Height
			wd := st.Width
			if limit := heightForBitrate(bw); limit > 0 && h > limit {
				wd, h = wd*limit/h, limit
			}
			res = fmt.Sprintf(",RESOLUTION=%dx%d", wd, h)
			break
		}
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	fmt.Fprintf(w, "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=%d,AVERAGE-BANDWIDTH=%d,CODECS=\"avc1.640028,mp4a.40.2\"%s\nmain.m3u8?%s\n", bw, bw, res, r.URL.RawQuery)
	return nil
}

func heightForBitrate(bps int64) int {
	switch {
	case bps <= 0 || bps >= 20_000_000:
		return 0
	case bps >= 8_000_000:
		return 1080
	case bps >= 3_000_000:
		return 720
	case bps >= 1_000_000:
		return 480
	}
	return 360
}

func (s *Server) jfMainPlaylist(w http.ResponseWriter, r *http.Request) error {
	it, err := s.DB.GetItem(normID(jfParam(r, "itemId")))
	if err != nil {
		return err
	}
	if it.Duration <= 0 {
		return errf(http.StatusBadRequest, "unknown duration")
	}
	n := int(math.Ceil(it.Duration / jfSegment))
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	var b strings.Builder
	fmt.Fprintf(&b, "#EXTM3U\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:%d\n#EXT-X-MEDIA-SEQUENCE:0\n", jfSegment)
	for i := 0; i < n; i++ {
		d := float64(jfSegment)
		if i == n-1 {
			d = it.Duration - float64(i*jfSegment)
		}
		fmt.Fprintf(&b, "#EXTINF:%.6f, nodesc\nhls1/main/%d.ts?%s\n", d, i, r.URL.RawQuery)
	}
	b.WriteString("#EXT-X-ENDLIST\n")
	_, err = w.Write([]byte(b.String()))
	return err
}

func (s *Server) jfSegment(w http.ResponseWriter, r *http.Request) error {
	it, err := s.DB.GetItem(normID(jfParam(r, "itemId")))
	if err != nil {
		return err
	}
	seg, err := strconv.Atoi(strings.TrimSuffix(jfParam(r, "segmentId"), ".ts"))
	if err != nil || seg < 0 {
		return errf(http.StatusBadRequest, "bad segment")
	}
	u := userFrom(r)
	play := jfQuery(r, "PlaySessionId")
	owner := u.ID + db.StableID(play)[:8]
	name := fmt.Sprintf("seg%05d.ts", seg)
	sess := s.Streams.Find(owner, it.ID)
	// Reuse the running job when the segment is (or will soon be) produced by it.
	if sess != nil {
		latest := sess.LatestSegment()
		if !(sess.HasSegment(name) || (seg >= sess.StartNumber && seg <= latest+3 && !sess.Done())) {
			sess = nil
		}
	}
	if sess == nil {
		opt := stream.Options{Start: float64(seg * jfSegment), AudioStream: -1, BurnSubtitle: -1, SegmentLength: jfSegment, StartNumber: seg, VOD: true}
		if v, err := strconv.Atoi(jfQuery(r, "AudioStreamIndex")); err == nil {
			opt.AudioStream = v
		}
		if v, err := strconv.Atoi(jfQuery(r, "SubtitleStreamIndex")); err == nil {
			opt.BurnSubtitle = v
		}
		if v, err := strconv.ParseInt(jfQuery(r, "VideoBitrate"), 10, 64); err == nil && v > 0 {
			opt.MaxBitrate = int(v / 1000)
			opt.MaxHeight = heightForBitrate(v)
		}
		if sess, err = s.Streams.Start(it, opt, owner); err != nil {
			return err
		}
	}
	ctx := r.Context()
	p, err := sess.WaitSegment(ctx, name)
	if err != nil {
		return errf(http.StatusNotFound, "segment not available")
	}
	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, p)
	return nil
}

// ---- Subtitles ----

func (s *Server) jfSubtitle(w http.ResponseWriter, r *http.Request) error {
	it, err := s.DB.GetItem(normID(jfParam(r, "itemId")))
	if err != nil {
		return err
	}
	idx, err := strconv.Atoi(jfParam(r, "index"))
	if err != nil {
		return errf(http.StatusBadRequest, "bad index")
	}
	format := strings.ToLower(strings.TrimPrefix(filepath.Ext(jfParam(r, "file")), "."))
	maxIdx := -1
	for _, st := range it.Media.Streams {
		if st.Index > maxIdx {
			maxIdx = st.Index
		}
		if st.Type == "subtitle" && st.Index == idx {
			if s.FFmpeg == "" {
				return errf(http.StatusNotImplemented, "ffmpeg required")
			}
			w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
			return stream.ExtractEmbedded(r.Context(), s.FFmpeg, it.Path, st.Index, w)
		}
	}
	ex := idx - maxIdx - 1
	if ex < 0 || ex >= len(it.Subtitles) {
		return errf(http.StatusNotFound, "no such subtitle")
	}
	sub := it.Subtitles[ex]
	if format == sub.Format {
		http.ServeFile(w, r, sub.Path)
		return nil
	}
	vtt, err := stream.ExternalToVTT(sub.Path, sub.Format)
	if err != nil {
		return errf(http.StatusUnsupportedMediaType, err.Error())
	}
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	_, err = w.Write([]byte(vtt))
	return err
}

// ---- Play state ----

type jfPlayState struct {
	ItemID        string `json:"ItemId"`
	PositionTicks int64
	IsPaused      bool
	PlaySessionID string `json:"PlaySessionId"`
	PlayMethod    string
}

func (s *Server) jfPlaying(event string) handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		var body jfPlayState
		_ = decode(r, &body)
		id := normID(body.ItemID)
		if id == "" {
			id = normID(jfParam(r, "itemId"))
		}
		if body.PositionTicks == 0 {
			if v, err := strconv.ParseInt(jfQuery(r, "PositionTicks"), 10, 64); err == nil {
				body.PositionTicks = v
			}
		}
		it, err := s.DB.GetItem(id)
		if err != nil {
			return noContent(w)
		}
		u := userFrom(r)
		a := parseJFAuth(r)
		method := "DirectPlay"
		if strings.EqualFold(body.PlayMethod, "Transcode") {
			method = "Transcode"
		}
		device := a.Device
		if a.Client != "" {
			device = a.Client + " · " + a.Device
		}
		if _, err := s.recordProgress(u, it, fromTicks(body.PositionTicks), 0, body.IsPaused, event, method, device); err != nil {
			return err
		}
		if event == "stop" && body.PlaySessionID != "" {
			if sess := s.Streams.Find(u.ID+db.StableID(body.PlaySessionID)[:8], it.ID); sess != nil {
				s.Streams.Stop(sess.ID)
			}
		}
		return noContent(w)
	}
}

func (s *Server) jfUserDataResponse(w http.ResponseWriter, it *db.Item, d db.UserData) error {
	return jfJSON(w, jfUserData(it, d, [2]int{}, false))
}

func (s *Server) jfMarkPlayed(played bool) handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		it, err := s.DB.GetItem(normID(jfParam(r, "itemId")))
		if err != nil {
			return err
		}
		d, err := s.applyPlayed(userFrom(r), it, played)
		if err != nil {
			return err
		}
		return s.jfUserDataResponse(w, it, d)
	}
}

func (s *Server) jfMarkFavorite(fav bool) handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		it, err := s.DB.GetItem(normID(jfParam(r, "itemId")))
		if err != nil {
			return err
		}
		d, err := s.applyFavorite(userFrom(r), it.ID, fav)
		if err != nil {
			return err
		}
		return s.jfUserDataResponse(w, it, d)
	}
}

func (s *Server) jfItemUserData(w http.ResponseWriter, r *http.Request) error {
	it, err := s.DB.GetItem(normID(jfParam(r, "itemId")))
	if err != nil {
		return err
	}
	uds, err := s.DB.GetUserData(userFrom(r).ID, []string{it.ID})
	if err != nil {
		return err
	}
	return s.jfUserDataResponse(w, it, uds[it.ID])
}

// ---- Misc stubs used by clients ----

func (s *Server) jfDisplayPrefs(w http.ResponseWriter, r *http.Request) error {
	u := userFrom(r)
	id, client := jfParam(r, "id"), jfQuery(r, "client")
	if r.Method == http.MethodPost {
		var body map[string]any
		if err := decode(r, &body); err != nil {
			return err
		}
		b, _ := json.Marshal(body)
		_, err := s.DB.Exec(`INSERT INTO display_prefs(user_id,pref_id,client,data) VALUES(?,?,?,?) ON CONFLICT(user_id,pref_id,client) DO UPDATE SET data=excluded.data`, u.ID, id, client, string(b))
		if err != nil {
			return err
		}
		return noContent(w)
	}
	var data string
	if err := s.DB.QueryRow(`SELECT data FROM display_prefs WHERE user_id=? AND pref_id=? AND client=?`, u.ID, id, client).Scan(&data); err == nil {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(data))
		return err
	}
	return jfJSON(w, map[string]any{
		"Id": id, "SortBy": "SortName", "RememberIndexing": false, "PrimaryImageHeight": 250, "PrimaryImageWidth": 250,
		"CustomPrefs": map[string]string{}, "ScrollDirection": "Horizontal", "ShowBackdrop": true, "RememberSorting": false,
		"SortOrder": "Ascending", "ShowSidebar": false, "Client": client,
	})
}

func (s *Server) jfEmpty(v any) handler {
	return func(w http.ResponseWriter, r *http.Request) error { return jfJSON(w, v) }
}

func (s *Server) jfNoContent(w http.ResponseWriter, r *http.Request) error { return noContent(w) }

func (s *Server) jfEndpoint(w http.ResponseWriter, r *http.Request) error {
	return jfJSON(w, map[string]any{"IsLocal": true, "IsInNetwork": true})
}

func (s *Server) jfPing(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Content-Type", "text/plain")
	_, err := w.Write([]byte("Jellyfin Server"))
	return err
}
