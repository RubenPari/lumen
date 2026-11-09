package scanner

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rubenpari/lumen/internal/db"
	"github.com/rubenpari/lumen/internal/metadata"
)

func newTestScanner(t *testing.T) (*Scanner, *db.DB) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	return &Scanner{DB: d, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), TMDB: func() *metadata.TMDB { return nil }}, d
}

func count(t *testing.T, d *db.DB, typ string) int {
	t.Helper()
	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM items WHERE type=?`, typ).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

func TestWatcherRealtime(t *testing.T) {
	s, d := newTestScanner(t)
	root := t.TempDir()
	lib := &db.Library{ID: "lib", Name: "Movies", Type: "movies", Paths: []string{root}, Realtime: true, CreatedAt: 1}
	if err := d.SaveLibrary(lib); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w, err := NewWatcher(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	w.Debounce = 100 * time.Millisecond
	w.Sync()

	dir := filepath.Join(root, "Dune (2021)")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Give the watcher a moment to add the new directory, then add the file.
	time.Sleep(150 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(dir, "Dune (2021).mkv"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "movie added", func() bool { return count(t, d, db.TypeMovie) == 1 })
	it, _ := d.QueryItems(`WHERE type='Movie'`)
	if it[0].Name != "Dune" || it[0].Year != 2021 {
		t.Fatalf("unexpected %s %d", it[0].Name, it[0].Year)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "movie removed", func() bool { return count(t, d, db.TypeMovie) == 0 })
}

func TestScanPathsOnlyTouchesUnit(t *testing.T) {
	s, d := newTestScanner(t)
	root := t.TempDir()
	for _, f := range []string{"Show A/Season 1/Show A S01E01.mkv", "Show B/Season 1/Show B S01E01.mkv"} {
		p := filepath.Join(root, f)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte("x"), 0o644)
	}
	lib := &db.Library{ID: "tv", Name: "TV", Type: "tvshows", Paths: []string{root}, CreatedAt: 1}
	_ = d.SaveLibrary(lib)
	if err := s.scanLibrary(context.Background(), lib); err != nil {
		t.Fatal(err)
	}
	if count(t, d, db.TypeEpisode) != 2 {
		t.Fatal("expected 2 episodes")
	}
	_ = os.WriteFile(filepath.Join(root, "Show A/Season 1/Show A S01E02.mkv"), []byte("x"), 0o644)
	_ = os.RemoveAll(filepath.Join(root, "Show B"))
	// Only Show A is rescanned: Show B must survive until its own unit is scanned.
	if err := s.ScanPaths(context.Background(), lib, []string{filepath.Join(root, "Show A")}); err != nil {
		t.Fatal(err)
	}
	if n := count(t, d, db.TypeEpisode); n != 3 {
		t.Fatalf("expected 3 episodes, got %d", n)
	}
	_ = s.ScanPaths(context.Background(), lib, []string{filepath.Join(root, "Show B")})
	if n := count(t, d, db.TypeSeries); n != 1 {
		t.Fatalf("expected 1 series, got %d", n)
	}
}

func TestDiscFolders(t *testing.T) {
	s, d := newTestScanner(t)
	root := t.TempDir()
	for _, f := range []string{"Old Movie (1990)/VIDEO_TS/VIDEO_TS.IFO", "Old Movie (1990)/VIDEO_TS/VTS_01_1.VOB", "HD Movie (2010)/BDMV/index.bdmv"} {
		p := filepath.Join(root, f)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte("x"), 0o644)
	}
	lib := &db.Library{ID: "m", Name: "M", Type: "movies", Paths: []string{root}, CreatedAt: 1}
	_ = d.SaveLibrary(lib)
	if err := s.scanLibrary(context.Background(), lib); err != nil {
		t.Fatal(err)
	}
	items, _ := d.QueryItems(`WHERE type='Movie' ORDER BY name`)
	if len(items) != 2 || items[0].Name != "HD Movie" || items[0].DiscType != "bluray" || items[1].DiscType != "dvd" || items[1].Year != 1990 {
		for _, i := range items {
			t.Logf("%s %d %s", i.Name, i.Year, i.DiscType)
		}
		t.Fatal("unexpected disc items")
	}
}
