package scanner

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rubenpari/lumen/internal/db"
)

// Watcher triggers targeted rescans when files change inside library folders
// (equivalent of Jellyfin's LibraryMonitor).
type Watcher struct {
	S        *Scanner
	Debounce time.Duration

	mu      sync.Mutex
	w       *fsnotify.Watcher
	roots   map[string]*db.Library // root path -> library
	pending map[string]map[string]bool
	timers  map[string]*time.Timer
	ctx     context.Context
}

// NewWatcher creates a watcher; call Sync to (re)load the libraries.
func NewWatcher(ctx context.Context, s *Scanner) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &Watcher{S: s, Debounce: 5 * time.Second, w: fw, ctx: ctx,
		roots: map[string]*db.Library{}, pending: map[string]map[string]bool{}, timers: map[string]*time.Timer{}}
	go w.loop()
	go func() {
		<-ctx.Done()
		fw.Close()
	}()
	return w, nil
}

// Sync updates the watched folders from the libraries in the database.
func (w *Watcher) Sync() {
	libs, err := w.S.DB.ListLibraries()
	if err != nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, p := range w.w.WatchList() {
		_ = w.w.Remove(p)
	}
	w.roots = map[string]*db.Library{}
	for _, l := range libs {
		if !l.Realtime {
			continue
		}
		for _, root := range l.Paths {
			w.roots[root] = l
			w.addTree(root)
		}
	}
}

// addTree adds a watch on dir and every subfolder (inotify is not recursive).
func (w *Watcher) addTree(dir string) {
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if p != dir && hidden(d.Name()) {
			return filepath.SkipDir
		}
		if err := w.w.Add(p); err != nil {
			if errors.Is(err, syscall.ENOSPC) {
				w.S.Log.Warn("inotify watch limit reached; increase fs.inotify.max_user_watches. Periodic scans still work.")
				return filepath.SkipAll
			}
		}
		return nil
	})
}

func (w *Watcher) loop() {
	for {
		select {
		case ev, ok := <-w.w.Events:
			if !ok {
				return
			}
			w.handle(ev)
		case err, ok := <-w.w.Errors:
			if !ok {
				return
			}
			w.S.Log.Warn("watcher", "err", err)
		}
	}
}

func (w *Watcher) handle(ev fsnotify.Event) {
	name := filepath.Base(ev.Name)
	if hidden(name) || strings.HasSuffix(name, ".tmp") || strings.HasSuffix(name, ".part") || strings.HasSuffix(name, ".!qB") {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if ev.Has(fsnotify.Create) {
		if fi, err := os.Stat(ev.Name); err == nil && fi.IsDir() {
			w.addTree(ev.Name)
		}
	}
	for root, lib := range w.roots {
		if !under(ev.Name, root) || ev.Name == root {
			continue
		}
		rel, err := filepath.Rel(root, ev.Name)
		if err != nil {
			continue
		}
		unit := filepath.Join(root, strings.Split(rel, string(filepath.Separator))[0])
		if w.pending[lib.ID] == nil {
			w.pending[lib.ID] = map[string]bool{}
		}
		w.pending[lib.ID][unit] = true
		if t := w.timers[lib.ID]; t != nil {
			t.Stop()
		}
		libID := lib.ID
		w.timers[libID] = time.AfterFunc(w.Debounce, func() { w.flush(libID) })
	}
}

func (w *Watcher) flush(libID string) {
	w.mu.Lock()
	units := w.pending[libID]
	delete(w.pending, libID)
	delete(w.timers, libID)
	w.mu.Unlock()
	if len(units) == 0 || w.ctx.Err() != nil {
		return
	}
	lib, err := w.S.DB.GetLibrary(libID)
	if err != nil {
		return
	}
	list := make([]string, 0, len(units))
	for u := range units {
		list = append(list, u)
	}
	if err := w.S.ScanPaths(w.ctx, lib, list); err != nil {
		w.S.Log.Error("realtime rescan", "err", err)
	}
}
