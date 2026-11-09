package api

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rubenpari/lumen/internal/db"
	"github.com/rubenpari/lumen/internal/livetv"
	"github.com/rubenpari/lumen/internal/metadata"
	"github.com/rubenpari/lumen/internal/stream"
)

type liveChannelDTO struct {
	livetv.ChannelRow
	Favorite bool `json:"favorite"`
}

func (s *Server) liveChannels(r *http.Request, rows []livetv.ChannelRow) ([]liveChannelDTO, error) {
	ids := make([]string, len(rows))
	for i, c := range rows {
		ids[i] = c.ID
	}
	ud, err := s.DB.GetUserData(userFrom(r).ID, ids)
	if err != nil {
		return nil, err
	}
	out := make([]liveChannelDTO, len(rows))
	for i, c := range rows {
		out[i] = liveChannelDTO{c, ud[c.ID].Favorite}
	}
	return out, nil
}

func (s *Server) listLiveChannels(w http.ResponseWriter, r *http.Request) error {
	rows, err := s.LiveTV.Channels()
	if err != nil {
		return err
	}
	out, err := s.liveChannels(r, rows)
	if err != nil {
		return err
	}
	return ok(w, out)
}

func (s *Server) liveGuide(w http.ResponseWriter, r *http.Request) error {
	from := qint(r, "from", 0)
	if from == 0 {
		from = int(time.Now().Add(-time.Hour).Unix())
	}
	hours := qint(r, "hours", 12)
	if hours <= 0 || hours > 72 {
		hours = 12
	}
	rows, err := s.LiveTV.Guide(int64(from), int64(from+hours*3600))
	if err != nil {
		return err
	}
	out, err := s.liveChannels(r, rows)
	if err != nil {
		return err
	}
	return ok(w, out)
}

func (s *Server) liveFavorite(fav bool) handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		if _, err := s.LiveTV.Channel(r.PathValue("id")); err != nil {
			return err
		}
		d, err := s.applyFavorite(userFrom(r), r.PathValue("id"), fav)
		if err != nil {
			return err
		}
		return ok(w, d)
	}
}

var liveProbeCache sync.Map // channel id -> cachedProbe

type cachedProbe struct {
	media db.MediaInfo
	at    time.Time
}

func liveInputOptions(c livetv.ChannelRow) []string {
	var opts []string
	if strings.HasPrefix(c.URL, "/") || strings.HasPrefix(c.URL, "file://") {
		// Local files (handy for testing) are read in real time and looped.
		return []string{"-re", "-stream_loop", "-1"}
	}
	if strings.HasPrefix(c.URL, "http") {
		opts = append(opts, "-reconnect", "1", "-reconnect_streamed", "1", "-reconnect_delay_max", "5")
		if c.UserAgent != "" {
			opts = append(opts, "-user_agent", c.UserAgent)
		}
	}
	return opts
}

// liveItem builds a pseudo item for a channel, probing its codecs once.
func (s *Server) liveItem(ctx context.Context, c livetv.ChannelRow) *db.Item {
	it := &db.Item{ID: c.ID, Name: c.Name, Path: strings.TrimPrefix(c.URL, "file://")}
	if v, ok := liveProbeCache.Load(c.ID); ok && time.Since(v.(cachedProbe).at) < time.Hour {
		it.Media = v.(cachedProbe).media
		return it
	}
	if s.FFprobe != "" {
		pctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		var extra []string
		if c.UserAgent != "" {
			extra = []string{"-user_agent", c.UserAgent}
		}
		if mi, _, err := metadata.Probe(pctx, s.FFprobe, it.Path, extra...); err == nil {
			it.Media = mi
			liveProbeCache.Store(c.ID, cachedProbe{mi, time.Now()})
		}
	}
	return it
}

// startLive starts (or reuses) a live HLS session for a channel.
func (s *Server) startLive(ctx context.Context, owner string, c livetv.ChannelRow) (*stream.Session, error) {
	if sess := s.Streams.Find(owner, c.ID); sess != nil && !sess.Done() {
		return sess, nil
	}
	it := s.liveItem(ctx, c)
	opt := stream.Options{Live: true, AudioStream: -1, BurnSubtitle: -1, SegmentLength: 3, InputOptions: liveInputOptions(c)}
	sess, err := s.Streams.Start(it, opt, owner)
	if err != nil {
		return nil, err
	}
	wctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	if err := sess.WaitPlaylist(wctx); err != nil {
		s.Streams.Stop(sess.ID)
		return nil, err
	}
	return sess, nil
}

func (s *Server) liveHLS(w http.ResponseWriter, r *http.Request) error {
	if s.FFmpeg == "" {
		return errf(http.StatusNotImplemented, "Live TV requires ffmpeg on the server")
	}
	c, err := s.LiveTV.Channel(r.PathValue("id"))
	if err != nil {
		return err
	}
	sess, err := s.startLive(r.Context(), userFrom(r).ID, c)
	if err != nil {
		return errf(http.StatusBadGateway, "cannot open channel: "+err.Error())
	}
	return ok(w, map[string]any{"sessionId": sess.ID, "url": "/api/hls/" + sess.ID + "/index.m3u8"})
}

// ---- Administration ----

func (s *Server) liveSources(w http.ResponseWriter, r *http.Request) error {
	srcs, err := s.LiveTV.Sources()
	if err != nil {
		return err
	}
	return ok(w, srcs)
}

func (s *Server) addLiveSource(w http.ResponseWriter, r *http.Request) error {
	var body livetv.Source
	if err := decode(r, &body); err != nil {
		return err
	}
	src, err := s.LiveTV.AddSource(r.Context(), body)
	if err != nil && src.ID == "" {
		return errf(http.StatusBadRequest, err.Error())
	}
	srcs, _ := s.LiveTV.Sources()
	for _, x := range srcs {
		if x.ID == src.ID {
			return ok(w, x)
		}
	}
	return ok(w, src)
}

func (s *Server) deleteLiveSource(w http.ResponseWriter, r *http.Request) error {
	if err := s.LiveTV.DeleteSource(r.PathValue("id")); err != nil {
		return err
	}
	return ok(w, map[string]bool{"ok": true})
}

func (s *Server) refreshLive(w http.ResponseWriter, r *http.Request) error {
	s.LiveTV.RefreshAll(r.Context())
	return s.liveSources(w, r)
}
