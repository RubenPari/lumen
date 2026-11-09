// Package api exposes the HTTP API and serves the web client.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rubenpari/lumen/internal/db"
	"github.com/rubenpari/lumen/internal/events"
	"github.com/rubenpari/lumen/internal/metadata"
	"github.com/rubenpari/lumen/internal/scanner"
	"github.com/rubenpari/lumen/internal/stream"
)

// Version of the server.
const Version = "0.1.0"

// Server holds dependencies of the HTTP handlers.
type Server struct {
	DB        *db.DB
	Scanner   *scanner.Scanner
	Streams   *stream.Manager
	Images    *stream.ImageCache
	FFmpeg    string
	FFprobe   string
	CacheDir  string
	Log       *slog.Logger
	Web       fs.FS // built web client (may be nil)
	Events    *events.Hub
	Watcher   interface{ Sync() } // realtime library monitor (may be nil)
	startedAt time.Time

	playing sync.Map // userID -> *NowPlaying
}

// NowPlaying describes an active playback for the dashboard.
type NowPlaying struct {
	UserID    string  `json:"userId"`
	UserName  string  `json:"userName"`
	ItemID    string  `json:"itemId"`
	ItemName  string  `json:"itemName"`
	Series    string  `json:"seriesName,omitempty"`
	Position  float64 `json:"position"`
	Duration  float64 `json:"duration"`
	Paused    bool    `json:"paused"`
	Method    string  `json:"method"`
	Device    string  `json:"device"`
	UpdatedAt int64   `json:"updatedAt"`
}

// TMDB returns a configured TMDB client or nil.
func (s *Server) TMDB() *metadata.TMDB {
	key := s.DB.GetSetting("tmdb_api_key", "")
	if key == "" {
		return nil
	}
	return metadata.NewTMDB(key, s.DB.GetSetting("metadata_language", "en-US"), s.CacheDir)
}

type ctxKey int

const userKey ctxKey = 1

func userFrom(r *http.Request) *db.User {
	u, _ := r.Context().Value(userKey).(*db.User)
	return u
}

type handler func(w http.ResponseWriter, r *http.Request) error

type httpError struct {
	Code int
	Msg  string
}

func (e *httpError) Error() string { return e.Msg }

func errf(code int, msg string) error { return &httpError{code, msg} }

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func ok(w http.ResponseWriter, v any) error {
	writeJSON(w, http.StatusOK, v)
	return nil
}

func decode(r *http.Request, v any) error {
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20)).Decode(v); err != nil {
		return errf(http.StatusBadRequest, "invalid JSON body")
	}
	return nil
}

func (s *Server) wrap(h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := h(w, r)
		if err == nil {
			return
		}
		var he *httpError
		switch {
		case errors.As(err, &he):
			writeJSON(w, he.Code, map[string]string{"error": he.Msg})
		case errors.Is(err, db.ErrNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		case errors.Is(err, context.Canceled):
		default:
			s.Log.Error("request failed", "path", r.URL.Path, "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
	}
}

func qint(r *http.Request, key string, def int) int {
	if v, err := strconv.Atoi(r.URL.Query().Get(key)); err == nil {
		return v
	}
	return def
}

func qfloat(r *http.Request, key string) float64 {
	v, _ := strconv.ParseFloat(r.URL.Query().Get(key), 64)
	return v
}

// Handler builds the HTTP handler.
func (s *Server) Handler() http.Handler {
	s.startedAt = time.Now()
	mux := http.NewServeMux()
	pub := func(pattern string, h handler) { mux.HandleFunc(pattern, s.wrap(h)) }
	auth := func(pattern string, h handler) { mux.HandleFunc(pattern, s.wrap(s.requireUser(h))) }
	admin := func(pattern string, h handler) { mux.HandleFunc(pattern, s.wrap(s.requireAdmin(h))) }

	// System & auth
	pub("GET /api/system/info", s.systemInfo)
	pub("POST /api/setup", s.setup)
	pub("GET /api/users/public", s.publicUsers)
	pub("POST /api/auth/login", s.login)
	auth("POST /api/auth/logout", s.logout)
	auth("GET /api/me", s.me)
	auth("GET /api/events", s.eventStream)
	auth("POST /api/me/password", s.changePassword)

	// Browsing
	auth("GET /api/libraries", s.listLibraries)
	auth("GET /api/home", s.home)
	auth("GET /api/items", s.listItems)
	auth("GET /api/items/{id}", s.getItem)
	auth("GET /api/items/{id}/children", s.children)
	auth("GET /api/items/{id}/similar", s.similar)
	auth("GET /api/items/{id}/next", s.nextEpisode)
	auth("GET /api/genres", s.genres)
	auth("GET /api/search", s.search)
	auth("GET /api/items/{id}/images/{kind}", s.image)

	// User data
	auth("POST /api/items/{id}/played", s.setPlayed(true))
	auth("DELETE /api/items/{id}/played", s.setPlayed(false))
	auth("POST /api/items/{id}/favorite", s.setFavorite(true))
	auth("DELETE /api/items/{id}/favorite", s.setFavorite(false))
	auth("POST /api/items/{id}/progress", s.progress)

	// Playback
	auth("GET /api/items/{id}/playback", s.playbackInfo)
	auth("GET /api/items/{id}/stream", s.directStream)
	auth("POST /api/items/{id}/hls", s.startHLS)
	auth("GET /api/hls/{sid}/{file}", s.hlsFile)
	auth("DELETE /api/hls/{sid}", s.stopHLS)
	auth("GET /api/items/{id}/subtitles/{idx}", s.subtitle)
	auth("GET /api/items/{id}/file", s.fileItem)
	auth("GET /api/items/{id}/audio", s.transcodeAudio)
	auth("GET /api/items/{id}/pages", s.pages)
	auth("GET /api/items/{id}/pages/{n}", s.page)
	auth("PUT /api/items/{id}/reading", s.reading)

	// Administration
	admin("POST /api/libraries", s.createLibrary)
	admin("PATCH /api/libraries/{id}", s.updateLibrary)
	admin("DELETE /api/libraries/{id}", s.deleteLibrary)
	admin("POST /api/libraries/{id}/scan", s.scanLibrary)
	admin("POST /api/scan", s.scanAll)
	admin("GET /api/scan/status", s.scanStatus)
	admin("GET /api/admin/fs", s.browseFS)
	admin("GET /api/admin/dashboard", s.dashboard)
	admin("GET /api/admin/settings", s.getSettings)
	admin("PUT /api/admin/settings", s.putSettings)
	admin("GET /api/admin/users", s.listUsers)
	admin("POST /api/admin/users", s.createUser)
	admin("PATCH /api/admin/users/{id}", s.updateUser)
	admin("DELETE /api/admin/users/{id}", s.deleteUser)
	admin("PATCH /api/items/{id}", s.editItem)
	admin("POST /api/items/{id}/refresh", s.refreshItem)

	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown endpoint"})
	})
	mux.HandleFunc("/", s.serveWeb)
	return s.logRequests(mux)
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
		if strings.HasPrefix(r.URL.Path, "/api/") && !strings.HasPrefix(r.URL.Path, "/api/hls/") && !strings.Contains(r.URL.Path, "/images/") {
			s.Log.Debug("http", "method", r.Method, "path", r.URL.Path, "dur", time.Since(start))
		}
	})
}

// serveWeb serves the single page application with history fallback.
func (s *Server) serveWeb(w http.ResponseWriter, r *http.Request) {
	if s.Web == nil {
		http.Error(w, "web client not built: run `make build`", http.StatusNotFound)
		return
	}
	if _, err := fs.Stat(s.Web, "index.html"); err != nil {
		http.Error(w, "web client not built: run `make build` (or `cd web && npm ci && npm run build`) and rebuild the server", http.StatusNotFound)
		return
	}
	p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if p == "" {
		p = "index.html"
	}
	if f, err := s.Web.Open(p); err == nil {
		st, _ := f.Stat()
		f.Close()
		if st != nil && !st.IsDir() {
			if strings.HasPrefix(p, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			http.ServeFileFS(w, r, s.Web, p)
			return
		}
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFileFS(w, r, s.Web, "index.html")
}

func (s *Server) systemInfo(w http.ResponseWriter, r *http.Request) error {
	return ok(w, map[string]any{
		"serverName":    s.DB.GetSetting("server_name", "Lumen"),
		"version":       Version,
		"setupRequired": s.DB.CountUsers() == 0,
		"ffmpeg":        s.FFmpeg != "",
	})
}

// librariesChanged re-syncs the watcher and notifies clients.
func (s *Server) librariesChanged() {
	if s.Watcher != nil {
		s.Watcher.Sync()
	}
	s.Events.Publish(events.Event{Type: "library.changed"})
}

// eventStream pushes server events to the web client (Server-Sent Events).
func (s *Server) eventStream(w http.ResponseWriter, r *http.Request) error {
	fl, okf := w.(http.Flusher)
	if !okf {
		return errf(http.StatusInternalServerError, "streaming unsupported")
	}
	u := userFrom(r)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	ch, cancel := s.Events.Subscribe()
	defer cancel()
	fmt.Fprint(w, "retry: 3000\n\n")
	fl.Flush()
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return nil
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		case e, open := <-ch:
			if !open {
				return nil
			}
			if e.User != "" && e.User != u.ID {
				continue
			}
			b, _ := json.Marshal(e)
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
		}
	}
}
