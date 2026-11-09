package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rubenpari/lumen/internal/db"
	"github.com/rubenpari/lumen/internal/metadata"
	"github.com/rubenpari/lumen/internal/scanner"
	"github.com/rubenpari/lumen/internal/stream"
)

type client struct {
	t    *testing.T
	base string
	http *http.Client
}

func (c *client) do(method, path string, body any, out any) int {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, r)
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func touch(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestEndToEnd(t *testing.T) {
	dir := t.TempDir()
	media := filepath.Join(dir, "media")
	touch(t, filepath.Join(media, "movies", "Inception (2010)", "Inception (2010).mkv"))
	touch(t, filepath.Join(media, "movies", "Inception (2010)", "Inception (2010).en.srt"))
	touch(t, filepath.Join(media, "tv", "Severance", "Season 01", "Severance.S01E01.Good.News.About.Hell.mkv"))
	touch(t, filepath.Join(media, "tv", "Severance", "Season 01", "Severance.S01E02.mkv"))

	database, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := &Server{DB: database, Log: log, CacheDir: dir, Streams: stream.NewManager("", filepath.Join(dir, "tx"), log), Images: stream.NewImageCache(filepath.Join(dir, "img"))}
	s.Scanner = &scanner.Scanner{DB: database, Log: log, TMDB: func() *metadata.TMDB { return nil }}
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	jar, _ := cookiejar.New(nil)
	c := &client{t: t, base: ts.URL, http: &http.Client{Jar: jar}}

	var info map[string]any
	c.do("GET", "/api/system/info", nil, &info)
	if info["setupRequired"] != true {
		t.Fatalf("expected setup required: %v", info)
	}
	if code := c.do("GET", "/api/home", nil, nil); code != 401 {
		t.Fatalf("expected 401, got %d", code)
	}
	if code := c.do("POST", "/api/setup", map[string]string{"username": "admin", "password": "pw"}, nil); code != 200 {
		t.Fatalf("setup: %d", code)
	}
	if code := c.do("POST", "/api/setup", map[string]string{"username": "x", "password": "pw"}, nil); code != 409 {
		t.Fatalf("second setup should fail: %d", code)
	}
	for _, l := range []map[string]any{
		{"name": "Movies", "type": "movies", "paths": []string{filepath.Join(media, "movies")}},
		{"name": "TV", "type": "tvshows", "paths": []string{filepath.Join(media, "tv")}},
	} {
		if code := c.do("POST", "/api/libraries", l, nil); code != 200 {
			t.Fatalf("create library: %d", code)
		}
		// Wait for the background scan.
		for i := 0; i < 100 && s.Scanner.Status().Running; i++ {
			time.Sleep(20 * time.Millisecond)
		}
		time.Sleep(100 * time.Millisecond)
		for i := 0; i < 100 && s.Scanner.Status().Running; i++ {
			time.Sleep(20 * time.Millisecond)
		}
	}
	var list struct {
		Items []ItemDTO `json:"items"`
		Total int       `json:"total"`
	}
	c.do("GET", "/api/items?sort=name", nil, &list)
	if list.Total != 2 || list.Items[0].Name != "Inception" || list.Items[0].Year != 2010 || list.Items[1].Name != "Severance" {
		t.Fatalf("unexpected items: %+v", list)
	}
	series := list.Items[1]
	var eps struct {
		Items []ItemDTO `json:"items"`
	}
	c.do("GET", "/api/items?type=Episode&seriesId="+series.ID+"&sort=episode", nil, &eps)
	if len(eps.Items) != 2 || eps.Items[0].Name != "Good News About Hell" || eps.Items[1].Name != "Episode 2" {
		t.Fatalf("episodes: %+v", eps.Items)
	}
	// Watch episode 1 to the end -> next up should be episode 2.
	c.do("POST", "/api/items/"+eps.Items[0].ID+"/progress", map[string]any{"position": 3000, "duration": 3100, "event": "stop"}, nil)
	var home struct {
		Rows []struct {
			Kind  string    `json:"kind"`
			Items []ItemDTO `json:"items"`
		} `json:"rows"`
	}
	c.do("GET", "/api/home", nil, &home)
	found := false
	for _, r := range home.Rows {
		if r.Kind == "nextup" && len(r.Items) == 1 && r.Items[0].ID == eps.Items[1].ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("next up missing: %+v", home.Rows)
	}
	// Subtitles of the movie are converted to WebVTT.
	var pb struct {
		Subtitles []subtitleTrack `json:"subtitles"`
	}
	c.do("GET", "/api/items/"+list.Items[0].ID+"/playback", nil, &pb)
	if len(pb.Subtitles) != 1 || pb.Subtitles[0].Language != "eng" {
		t.Fatalf("subtitles: %+v", pb.Subtitles)
	}
	// Non-admin users cannot reach admin endpoints.
	c.do("POST", "/api/admin/users", map[string]any{"name": "kid", "password": "a"}, nil)
	jar2, _ := cookiejar.New(nil)
	kid := &client{t: t, base: ts.URL, http: &http.Client{Jar: jar2}}
	if code := kid.do("POST", "/api/auth/login", map[string]string{"username": "kid", "password": "a"}, nil); code != 200 {
		t.Fatalf("kid login: %d", code)
	}
	if code := kid.do("GET", "/api/admin/dashboard", nil, nil); code != 403 {
		t.Fatalf("expected 403, got %d", code)
	}
	if code := kid.do("POST", "/api/auth/login", map[string]string{"username": "kid", "password": "wrong"}, nil); code != 401 {
		t.Fatalf("expected 401, got %d", code)
	}
}
