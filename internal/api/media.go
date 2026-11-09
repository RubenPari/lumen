package api

import (
	"context"
	"mime"
	"net/http"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rubenpari/lumen/internal/db"
	"github.com/rubenpari/lumen/internal/events"
	"github.com/rubenpari/lumen/internal/metadata"
)

// fileItem serves the original file of a non-folder item (books, photos,
// audio) with Range support; ?download=1 forces a download.
func (s *Server) fileItem(w http.ResponseWriter, r *http.Request) error {
	it, err := s.DB.GetItem(r.PathValue("id"))
	if err != nil {
		return err
	}
	if it.IsFolder || it.DiscType != "" {
		return errf(http.StatusBadRequest, "not a file")
	}
	if ct := mime.TypeByExtension(strings.ToLower(filepath.Ext(it.Path))); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	switch strings.ToLower(filepath.Ext(it.Path)) {
	case ".epub":
		w.Header().Set("Content-Type", "application/epub+zip")
	case ".flac":
		w.Header().Set("Content-Type", "audio/flac")
	case ".m4b", ".m4a":
		w.Header().Set("Content-Type", "audio/mp4")
	case ".opus":
		w.Header().Set("Content-Type", "audio/ogg")
	}
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filepath.Base(it.Path)}))
	}
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeFile(w, r, it.Path)
	return nil
}

// transcodeAudio streams an audio file converted to MP3 (for formats the
// browser cannot decode). ?start= seeks in seconds.
func (s *Server) transcodeAudio(w http.ResponseWriter, r *http.Request) error {
	if s.FFmpeg == "" {
		return errf(http.StatusNotImplemented, "transcoding requires ffmpeg")
	}
	it, err := s.DB.GetItem(r.PathValue("id"))
	if err != nil {
		return err
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin"}
	if st := qfloat(r, "start"); st > 0 {
		args = append(args, "-ss", strconv.FormatFloat(st, 'f', 3, 64))
	}
	args = append(args, "-i", it.Path, "-vn", "-map", "0:a:0", "-c:a", "libmp3lame", "-b:a", "256k", "-f", "mp3", "pipe:1")
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Hour)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.FFmpeg, args...)
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Cache-Control", "no-cache")
	cmd.Stdout = w
	if err := cmd.Run(); err != nil && ctx.Err() == nil {
		s.Log.Warn("audio transcode", "item", it.Name, "err", err)
	}
	return nil
}

var pageCache sync.Map // item id + mtime -> []string

func (s *Server) comicPages(it *db.Item) ([]string, error) {
	key := it.ID + ":" + strconv.FormatInt(it.MTime, 10)
	if v, ok := pageCache.Load(key); ok {
		return v.([]string), nil
	}
	pages, err := metadata.ComicPages(it.Path)
	if err != nil {
		return nil, err
	}
	pageCache.Store(key, pages)
	return pages, nil
}

func (s *Server) pages(w http.ResponseWriter, r *http.Request) error {
	it, err := s.DB.GetItem(r.PathValue("id"))
	if err != nil {
		return err
	}
	pages, err := s.comicPages(it)
	if err != nil {
		return errf(http.StatusBadRequest, err.Error())
	}
	return ok(w, map[string]int{"count": len(pages)})
}

func (s *Server) page(w http.ResponseWriter, r *http.Request) error {
	it, err := s.DB.GetItem(r.PathValue("id"))
	if err != nil {
		return err
	}
	pages, err := s.comicPages(it)
	if err != nil {
		return errf(http.StatusBadRequest, err.Error())
	}
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil || n < 0 || n >= len(pages) {
		return errf(http.StatusNotFound, "no such page")
	}
	data, err := metadata.ComicPage(it.Path, pages[n])
	if err != nil {
		return err
	}
	if ct := mime.TypeByExtension(strings.ToLower(filepath.Ext(pages[n]))); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Cache-Control", "private, max-age=86400")
	_, err = w.Write(data)
	return err
}

// reading saves the reading position of a book: percent (0-100) and an opaque
// location string (EPUB CFI or page number).
func (s *Server) reading(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Percent  float64 `json:"percent"`
		Location string  `json:"location"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	u := userFrom(r)
	it, err := s.DB.GetItem(r.PathValue("id"))
	if err != nil {
		return err
	}
	uds, err := s.DB.GetUserData(u.ID, []string{it.ID})
	if err != nil {
		return err
	}
	d := uds[it.ID]
	d.LastPlayed = db.Now()
	d.ProgressText = body.Location
	if body.Percent >= 99 {
		if !d.Played {
			d.PlayCount++
		}
		d.Played, d.Position = true, 0
	} else {
		// Position holds the percentage for books (duration is treated as 100).
		d.Position = max(body.Percent, 0.01)
		d.Played = false
	}
	if err := s.DB.SaveUserData(u.ID, it.ID, d); err != nil {
		return err
	}
	s.Events.Publish(events.Event{Type: "userdata.changed", User: u.ID, Data: it.ID})
	return ok(w, d)
}
