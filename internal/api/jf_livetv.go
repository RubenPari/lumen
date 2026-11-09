package api

import (
	"bufio"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rubenpari/lumen/internal/livetv"
)

// Jellyfin-compatible Live TV endpoints (channels, programs, live streams).

func (s *Server) jfProgramDto(p livetv.ProgramRow, ch livetv.ChannelRow) map[string]any {
	return map[string]any{
		"Id": p.ID, "Name": p.Title, "EpisodeTitle": p.SubTitle, "Overview": p.Description, "Type": "Program", "MediaType": "Video",
		"ChannelId": ch.ID, "ChannelName": ch.Name, "ChannelNumber": ch.Number, "ServerId": s.serverID(),
		"StartDate": unixISO(p.Start), "EndDate": unixISO(p.Stop), "RunTimeTicks": toTicks(float64(p.Stop - p.Start)),
		"Genres": splitList(p.Category), "IsMovie": strings.Contains(strings.ToLower(p.Category), "film") || strings.Contains(strings.ToLower(p.Category), "movie"),
		"IsNews": strings.Contains(strings.ToLower(p.Category), "news"), "IsSeries": p.Episode != "", "IsLive": false,
		"ImageTags": map[string]string{}, "BackdropImageTags": []string{}, "LocationType": "Remote",
	}
}

func (s *Server) jfChannelDto(ch livetv.ChannelRow, fav bool) map[string]any {
	d := map[string]any{
		"Id": ch.ID, "Name": ch.Name, "Number": ch.Number, "ChannelNumber": ch.Number, "ServerId": s.serverID(), "Type": "TvChannel",
		"ChannelType": "TV", "MediaType": "Video", "IsFolder": false, "LocationType": "Remote", "ImageTags": map[string]string{},
		"BackdropImageTags": []string{}, "SortName": ch.Number,
		"UserData": map[string]any{"IsFavorite": fav, "Played": false, "PlaybackPositionTicks": 0, "PlayCount": 0, "Key": ch.ID, "ItemId": ch.ID},
	}
	if ch.Logo != "" {
		d["ImageTags"] = map[string]string{"Primary": imgTag(ch.Logo)}
	}
	if ch.Now != nil {
		d["CurrentProgram"] = s.jfProgramDto(*ch.Now, ch)
	}
	return d
}

func (s *Server) jfLiveInfo(w http.ResponseWriter, r *http.Request) error {
	return jfJSON(w, map[string]any{
		"Services":     []any{map[string]any{"Name": "Lumen", "HomePageUrl": "", "Status": "Ok", "StatusMessage": "", "Version": Version, "HasUpdateAvailable": false, "IsVisible": true, "Tuners": []any{}}},
		"IsEnabled":    s.liveTVChannelCount() > 0,
		"EnabledUsers": []any{userFrom(r).ID},
	})
}

func (s *Server) jfLiveChannels(w http.ResponseWriter, r *http.Request) error {
	rows, err := s.LiveTV.Channels()
	if err != nil {
		return err
	}
	dtos, err := s.liveChannels(r, rows)
	if err != nil {
		return err
	}
	onlyFav := strings.EqualFold(jfQuery(r, "IsFavorite"), "true")
	items := []any{}
	for _, c := range dtos {
		if onlyFav && !c.Favorite {
			continue
		}
		items = append(items, s.jfChannelDto(c.ChannelRow, c.Favorite))
	}
	total := len(items)
	start, _ := strconv.Atoi(jfQuery(r, "StartIndex"))
	if start > 0 && start < len(items) {
		items = items[start:]
	}
	if lim, err := strconv.Atoi(jfQuery(r, "Limit")); err == nil && lim > 0 && lim < len(items) {
		items = items[:lim]
	}
	return jfJSON(w, map[string]any{"Items": items, "TotalRecordCount": total, "StartIndex": start})
}

func (s *Server) jfLiveChannel(w http.ResponseWriter, r *http.Request) error {
	ch, err := s.LiveTV.Channel(normID(jfParam(r, "channelId")))
	if err != nil {
		return err
	}
	return jfJSON(w, s.jfChannelDto(ch, false))
}

func parseISO(v string) int64 {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.0000000Z", "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t.Unix()
		}
	}
	return 0
}

func (s *Server) jfLivePrograms(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		ChannelIds   []string
		MinEndDate   string
		MaxStartDate string
		IsAiring     *bool
		Limit        int
	}
	if r.Method == http.MethodPost {
		_ = decode(r, &body)
	}
	if v := jfQuery(r, "ChannelIds"); v != "" {
		body.ChannelIds = splitList(v)
	}
	if v := jfQuery(r, "MinEndDate"); v != "" {
		body.MinEndDate = v
	}
	if v := jfQuery(r, "MaxStartDate"); v != "" {
		body.MaxStartDate = v
	}
	if v := jfQuery(r, "IsAiring"); v != "" {
		b := strings.EqualFold(v, "true")
		body.IsAiring = &b
	}
	if v, err := strconv.Atoi(jfQuery(r, "Limit")); err == nil {
		body.Limit = v
	}
	now := time.Now().Unix()
	from, to := now-3600, now+24*3600
	if v := parseISO(body.MinEndDate); v > 0 {
		from = v
	}
	if v := parseISO(body.MaxStartDate); v > 0 {
		to = v
	}
	if body.IsAiring != nil && *body.IsAiring {
		from, to = now, now+1
	}
	guide, err := s.LiveTV.Guide(from, to)
	if err != nil {
		return err
	}
	want := map[string]bool{}
	for _, id := range body.ChannelIds {
		want[normID(id)] = true
	}
	items := []any{}
	for _, ch := range guide {
		if len(want) > 0 && !want[ch.ID] {
			continue
		}
		for _, p := range ch.Programs {
			items = append(items, s.jfProgramDto(p, ch))
			if body.Limit > 0 && len(items) >= body.Limit {
				break
			}
		}
	}
	return jfJSON(w, map[string]any{"Items": items, "TotalRecordCount": len(items), "StartIndex": 0})
}

// jfLivePlaybackInfo describes a channel as an infinite HLS stream.
func (s *Server) jfLivePlaybackInfo(w http.ResponseWriter, r *http.Request, ch livetv.ChannelRow) error {
	src := map[string]any{
		"Id": ch.ID, "Name": ch.Name, "Protocol": "Http", "Type": "Default", "Container": "ts", "IsRemote": true, "IsInfiniteStream": true,
		"SupportsDirectPlay": false, "SupportsDirectStream": false, "SupportsTranscoding": true, "RequiresOpening": false,
		"MediaStreams": []any{}, "MediaAttachments": []any{}, "Formats": []any{}, "RequiredHttpHeaders": map[string]string{},
		"TranscodingUrl":         "/LiveTv/Channels/" + ch.ID + "/live.m3u8?api_key=" + parseJFAuth(r).Token,
		"TranscodingSubProtocol": "hls", "TranscodingContainer": "ts",
	}
	return jfJSON(w, map[string]any{"MediaSources": []any{src}, "PlaySessionId": ch.ID})
}

// jfLivePlaylist starts the channel and serves its playlist with rewritten segment URLs.
func (s *Server) jfLivePlaylist(w http.ResponseWriter, r *http.Request) error {
	ch, err := s.LiveTV.Channel(normID(jfParam(r, "channelId")))
	if err != nil {
		return err
	}
	sess, err := s.startLive(r.Context(), userFrom(r).ID, ch)
	if err != nil {
		return errf(http.StatusBadGateway, err.Error())
	}
	f, err := os.Open(filepath.Join(sess.Dir, "index.m3u8"))
	if err != nil {
		return err
	}
	defer f.Close()
	token := parseJFAuth(r).Token
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-cache")
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasSuffix(line, ".ts") && !strings.HasPrefix(line, "#") {
			line = "live/" + line + "?api_key=" + token
		}
		_, _ = w.Write([]byte(line + "\n"))
	}
	return nil
}

func (s *Server) jfLiveSegment(w http.ResponseWriter, r *http.Request) error {
	ch, err := s.LiveTV.Channel(normID(jfParam(r, "channelId")))
	if err != nil {
		return err
	}
	sess := s.Streams.Find(userFrom(r).ID, ch.ID)
	if sess == nil {
		return errf(http.StatusNotFound, "stream not running")
	}
	p, err := sess.WaitSegment(r.Context(), jfParam(r, "file"))
	if err != nil {
		return errf(http.StatusNotFound, "segment not available")
	}
	w.Header().Set("Content-Type", "video/mp2t")
	http.ServeFile(w, r, p)
	return nil
}
