// Command lumen is a self-hosted media server inspired by Jellyfin.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rubenpari/lumen/internal/api"
	"github.com/rubenpari/lumen/internal/db"
	"github.com/rubenpari/lumen/internal/events"
	"github.com/rubenpari/lumen/internal/livetv"
	"github.com/rubenpari/lumen/internal/metadata"
	"github.com/rubenpari/lumen/internal/scanner"
	"github.com/rubenpari/lumen/internal/stream"
	"github.com/rubenpari/lumen/web"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func defaultDataDir() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "lumen")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "lumen")
}

func findTool(explicit, name string) string {
	if explicit != "" {
		if explicit == "none" {
			return ""
		}
		return explicit
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	exe, _ := os.Executable()
	for _, c := range []string{filepath.Join(filepath.Dir(exe), name), "/usr/lib/jellyfin-ffmpeg/" + name} {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}
	return ""
}

func main() {
	addr := flag.String("addr", env("LUMEN_ADDR", ":8096"), "listen address")
	dataDir := flag.String("data", env("LUMEN_DATA_DIR", defaultDataDir()), "data directory")
	ffmpegPath := flag.String("ffmpeg", env("LUMEN_FFMPEG", ""), "path to ffmpeg (\"none\" to disable)")
	ffprobePath := flag.String("ffprobe", env("LUMEN_FFPROBE", ""), "path to ffprobe (\"none\" to disable)")
	debug := flag.Bool("debug", os.Getenv("LUMEN_DEBUG") != "", "debug logging")
	flag.Parse()

	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		log.Error("data dir", "err", err)
		os.Exit(1)
	}
	database, err := db.Open(filepath.Join(*dataDir, "lumen.db"))
	if err != nil {
		log.Error("open database", "err", err)
		os.Exit(1)
	}
	defer database.Close()

	ffmpeg := findTool(*ffmpegPath, "ffmpeg")
	ffprobe := findTool(*ffprobePath, "ffprobe")
	if ffmpeg == "" || ffprobe == "" {
		log.Warn("ffmpeg/ffprobe not found: transcoding and media probing are disabled; only direct play will work")
	}
	cacheDir := filepath.Join(*dataDir, "cache")
	streams := stream.NewManager(ffmpeg, filepath.Join(cacheDir, "transcodes"), log)
	streams.HWAccel = database.GetSetting("hw_accel", "")

	srv := &api.Server{
		DB: database, FFmpeg: ffmpeg, FFprobe: ffprobe, CacheDir: cacheDir, Log: log,
		Streams: streams, Images: stream.NewImageCache(filepath.Join(cacheDir, "images")), Web: web.FS(),
	}
	srv.Events = events.New()
	srv.Scanner = &scanner.Scanner{DB: database, FFprobe: ffprobe, Log: log, Events: srv.Events, CacheDir: cacheDir,
		TMDB: func() *metadata.TMDB { return srv.TMDB() }}
	srv.Scanner.OnChange = func() { srv.Events.Publish(events.Event{Type: "library.changed"}) }

	srv.LiveTV = livetv.New(database, log)
	srv.LiveTV.Lang = func() string { return strings.SplitN(database.GetSetting("metadata_language", "en-US"), "-", 2)[0] }
	srv.LiveTV.OnChange = func() { srv.Events.Publish(events.Event{Type: "livetv.changed"}) }

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if w, err := scanner.NewWatcher(ctx, srv.Scanner); err != nil {
		log.Warn("realtime monitoring disabled", "err", err)
	} else {
		w.Sync()
		srv.Watcher = w
	}

	go func() {
		srv.LiveTV.RefreshAll(ctx)
		srv.LiveTV.Run(ctx, 12*time.Hour)
	}()

	// Periodic library scan.
	go func() {
		last := time.Now()
		_ = srv.Scanner.ScanAll(ctx)
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				hours, _ := strconv.ParseFloat(database.GetSetting("scan_interval_hours", "12"), 64)
				if hours > 0 && time.Since(last) > time.Duration(hours*float64(time.Hour)) {
					last = time.Now()
					_ = srv.Scanner.ScanAll(ctx)
				}
			}
		}
	}()

	if _, port, err := net.SplitHostPort(*addr); err == nil && os.Getenv("LUMEN_NO_DISCOVERY") == "" {
		go srv.ServeDiscovery(ctx, port)
	}

	httpSrv := &http.Server{Addr: *addr, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(sctx)
	}()
	fmt.Fprintf(os.Stderr, "\n  Lumen %s listening on http://localhost%s  (data: %s)\n\n", api.Version, *addr, *dataDir)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("http", "err", err)
		os.Exit(1)
	}
}
