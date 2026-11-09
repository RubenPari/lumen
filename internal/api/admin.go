package api

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/rubenpari/lumen/internal/db"
	"github.com/rubenpari/lumen/internal/scanner"
)

type libraryBody struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Paths    []string `json:"paths"`
	Realtime *bool    `json:"realtime"`
}

func (b *libraryBody) validate() error {
	b.Name = strings.TrimSpace(b.Name)
	if b.Name == "" {
		return errf(http.StatusBadRequest, "name is required")
	}
	switch b.Type {
	case "movies", "tvshows", "music", "photos", "books":
	default:
		return errf(http.StatusBadRequest, "type must be one of movies, tvshows, music, photos, books")
	}
	var clean []string
	for _, p := range b.Paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !filepath.IsAbs(p) {
			return errf(http.StatusBadRequest, "paths must be absolute: "+p)
		}
		if fi, err := os.Stat(p); err != nil || !fi.IsDir() {
			return errf(http.StatusBadRequest, "folder not found: "+p)
		}
		clean = append(clean, filepath.Clean(p))
	}
	if len(clean) == 0 {
		return errf(http.StatusBadRequest, "at least one folder is required")
	}
	b.Paths = clean
	return nil
}

func (s *Server) createLibrary(w http.ResponseWriter, r *http.Request) error {
	var b libraryBody
	if err := decode(r, &b); err != nil {
		return err
	}
	if err := b.validate(); err != nil {
		return err
	}
	l := &db.Library{ID: db.NewID(), Name: b.Name, Type: b.Type, Paths: b.Paths, CreatedAt: db.Now(), Realtime: b.Realtime == nil || *b.Realtime}
	if err := s.DB.SaveLibrary(l); err != nil {
		return err
	}
	s.librariesChanged()
	_ = s.Scanner.ScanAll(context.Background(), l.ID)
	return ok(w, l)
}

func (s *Server) updateLibrary(w http.ResponseWriter, r *http.Request) error {
	l, err := s.DB.GetLibrary(r.PathValue("id"))
	if err != nil {
		return err
	}
	var b libraryBody
	if err := decode(r, &b); err != nil {
		return err
	}
	b.Type = l.Type // the content type cannot change
	if err := b.validate(); err != nil {
		return err
	}
	l.Name, l.Paths = b.Name, b.Paths
	if b.Realtime != nil {
		l.Realtime = *b.Realtime
	}
	if err := s.DB.SaveLibrary(l); err != nil {
		return err
	}
	s.librariesChanged()
	_ = s.Scanner.ScanAll(context.Background(), l.ID)
	return ok(w, l)
}

func (s *Server) deleteLibrary(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	if err := s.DB.DeleteLibrary(id); err != nil {
		return err
	}
	s.librariesChanged()
	return ok(w, map[string]bool{"ok": true})
}

func (s *Server) scanLibrary(w http.ResponseWriter, r *http.Request) error {
	if err := s.Scanner.ScanAll(context.Background(), r.PathValue("id")); err != nil {
		return errf(http.StatusConflict, err.Error())
	}
	return ok(w, s.Scanner.Status())
}

func (s *Server) scanAll(w http.ResponseWriter, r *http.Request) error {
	if err := s.Scanner.ScanAll(context.Background()); err != nil {
		return errf(http.StatusConflict, err.Error())
	}
	return ok(w, s.Scanner.Status())
}

func (s *Server) scanStatus(w http.ResponseWriter, r *http.Request) error {
	return ok(w, s.Scanner.Status())
}

func (s *Server) browseFS(w http.ResponseWriter, r *http.Request) error {
	p := r.URL.Query().Get("path")
	if p == "" {
		home, _ := os.UserHomeDir()
		roots := []map[string]string{{"name": "/", "path": "/"}}
		if home != "" {
			roots = append(roots, map[string]string{"name": "Home", "path": home})
		}
		for _, m := range []string{"/mnt", "/media", "/srv", "/data"} {
			if fi, err := os.Stat(m); err == nil && fi.IsDir() {
				roots = append(roots, map[string]string{"name": m, "path": m})
			}
		}
		return ok(w, map[string]any{"path": "", "parent": "", "dirs": roots})
	}
	p = filepath.Clean(p)
	entries, err := os.ReadDir(p)
	if err != nil {
		return errf(http.StatusBadRequest, err.Error())
	}
	dirs := []map[string]string{}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		full := filepath.Join(p, e.Name())
		isDir := e.IsDir()
		if e.Type()&os.ModeSymlink != 0 {
			if fi, err := os.Stat(full); err == nil {
				isDir = fi.IsDir()
			}
		}
		if isDir {
			dirs = append(dirs, map[string]string{"name": e.Name(), "path": full})
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return strings.ToLower(dirs[i]["name"]) < strings.ToLower(dirs[j]["name"]) })
	parent := filepath.Dir(p)
	if parent == p {
		parent = ""
	}
	return ok(w, map[string]any{"path": p, "parent": parent, "dirs": dirs})
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) error {
	totals, err := s.DB.MediaTotals()
	if err != nil {
		return err
	}
	playing := []*NowPlaying{}
	s.playing.Range(func(_, v any) bool {
		np := v.(*NowPlaying)
		if db.Now()-np.UpdatedAt < 60 {
			playing = append(playing, np)
		}
		return true
	})
	recent, _ := s.DB.QueryItems(`WHERE type IN ('Movie','Episode') AND extra_type='' ORDER BY date_added DESC LIMIT 8`)
	rd, err := s.dto(r, recent)
	if err != nil {
		return err
	}
	if rd == nil {
		rd = []*ItemDTO{}
	}
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	return ok(w, map[string]any{
		"counts":     totals.Counts,
		"totalSize":  totals.Size,
		"totalHours": totals.Duration / 3600,
		"playing":    playing,
		"transcodes": s.Streams.Count(),
		"scan":       s.Scanner.Status(),
		"version":    Version,
		"uptime":     int64(time.Since(s.startedAt).Seconds()),
		"ffmpeg":     s.FFmpeg,
		"ffprobe":    s.FFprobe,
		"goVersion":  runtime.Version(),
		"memoryMB":   mem.Alloc / 1024 / 1024,
		"recent":     rd,
		"users":      s.DB.CountUsers(),
	})
}

var settingKeys = []string{"server_name", "tmdb_api_key", "metadata_language", "scan_interval_hours", "hw_accel", "hide_users"}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) error {
	out := map[string]string{}
	defaults := map[string]string{"server_name": "Lumen", "metadata_language": "en-US", "scan_interval_hours": "12", "hw_accel": "", "hide_users": "false"}
	for _, k := range settingKeys {
		out[k] = s.DB.GetSetting(k, defaults[k])
	}
	return ok(w, out)
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) error {
	var body map[string]string
	if err := decode(r, &body); err != nil {
		return err
	}
	for _, k := range settingKeys {
		if v, found := body[k]; found {
			if err := s.DB.SetSetting(k, strings.TrimSpace(v)); err != nil {
				return err
			}
		}
	}
	s.Streams.HWAccel = s.DB.GetSetting("hw_accel", "")
	return s.getSettings(w, r)
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) error {
	users, err := s.DB.ListUsers()
	if err != nil {
		return err
	}
	if users == nil {
		users = []*db.User{}
	}
	return ok(w, users)
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Name     string `json:"name"`
		Password string `json:"password"`
		IsAdmin  bool   `json:"isAdmin"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	if !validName(body.Name) {
		return errf(http.StatusBadRequest, "invalid name")
	}
	if _, err := s.DB.GetUserByName(strings.TrimSpace(body.Name)); err == nil {
		return errf(http.StatusConflict, "a user with this name already exists")
	}
	hash, err := hashPassword(body.Password)
	if err != nil {
		return err
	}
	u := &db.User{ID: db.NewID(), Name: strings.TrimSpace(body.Name), PasswordHash: hash, IsAdmin: body.IsAdmin, CreatedAt: db.Now(), HasPassword: hash != ""}
	if err := s.DB.CreateUser(u); err != nil {
		return err
	}
	return ok(w, u)
}

func (s *Server) adminCount() (int, error) {
	return s.DB.CountAdmins()
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) error {
	u, err := s.DB.GetUser(r.PathValue("id"))
	if err != nil {
		return err
	}
	var body struct {
		Name     *string `json:"name"`
		Password *string `json:"password"`
		IsAdmin  *bool   `json:"isAdmin"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	if body.Name != nil {
		if !validName(*body.Name) {
			return errf(http.StatusBadRequest, "invalid name")
		}
		u.Name = strings.TrimSpace(*body.Name)
	}
	if body.IsAdmin != nil {
		if !*body.IsAdmin && u.IsAdmin {
			n, err := s.adminCount()
			if err != nil {
				return err
			}
			if n <= 1 {
				return errf(http.StatusBadRequest, "at least one administrator is required")
			}
		}
		u.IsAdmin = *body.IsAdmin
	}
	if body.Password != nil {
		if u.PasswordHash, err = hashPassword(*body.Password); err != nil {
			return err
		}
	}
	if err := s.DB.UpdateUser(u); err != nil {
		return err
	}
	u.HasPassword = u.PasswordHash != ""
	return ok(w, u)
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) error {
	u, err := s.DB.GetUser(r.PathValue("id"))
	if err != nil {
		return err
	}
	if u.ID == userFrom(r).ID {
		return errf(http.StatusBadRequest, "you cannot delete yourself")
	}
	if u.IsAdmin {
		n, err := s.adminCount()
		if err != nil {
			return err
		}
		if n <= 1 {
			return errf(http.StatusBadRequest, "at least one administrator is required")
		}
	}
	if err := s.DB.DeleteUser(u.ID); err != nil {
		return err
	}
	return ok(w, map[string]bool{"ok": true})
}

func (s *Server) editItem(w http.ResponseWriter, r *http.Request) error {
	it, err := s.DB.GetItem(r.PathValue("id"))
	if err != nil {
		return err
	}
	var body struct {
		Name           *string  `json:"name"`
		OriginalTitle  *string  `json:"originalTitle"`
		SortName       *string  `json:"sortName"`
		Overview       *string  `json:"overview"`
		Tagline        *string  `json:"tagline"`
		Year           *int     `json:"year"`
		OfficialRating *string  `json:"officialRating"`
		Rating         *float64 `json:"communityRating"`
		Genres         []string `json:"genres"`
		TmdbID         *string  `json:"tmdbId"`
		Locked         *bool    `json:"metadataLocked"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	set := func(dst *string, v *string) {
		if v != nil {
			*dst = strings.TrimSpace(*v)
		}
	}
	set(&it.Name, body.Name)
	set(&it.OriginalTitle, body.OriginalTitle)
	set(&it.SortName, body.SortName)
	set(&it.Overview, body.Overview)
	set(&it.Tagline, body.Tagline)
	set(&it.OfficialRating, body.OfficialRating)
	if body.SortName != nil && *body.SortName == "" {
		it.SortName = ""
	}
	if body.Year != nil {
		it.Year = *body.Year
	}
	if body.Rating != nil {
		it.CommunityRating = *body.Rating
	}
	if body.Genres != nil {
		it.Genres = body.Genres
	}
	tmdbChanged := false
	if body.TmdbID != nil && strings.TrimSpace(*body.TmdbID) != it.TmdbID {
		it.TmdbID = strings.TrimSpace(*body.TmdbID)
		tmdbChanged = true
	}
	// Editing locks the metadata so scans/refreshes do not overwrite it,
	// unless explicitly unlocked.
	it.MetadataLocked = true
	if body.Locked != nil {
		it.MetadataLocked = *body.Locked
	}
	locked := it.MetadataLocked
	// Upsert respects the old lock state; temporarily write with lock off.
	if err := s.DB.SetMetadataLocked(it.ID, false); err != nil {
		return err
	}
	it.MetadataLocked = false
	if err := s.DB.UpsertItem(it); err != nil {
		return err
	}
	if err := s.DB.SetMetadataLocked(it.ID, locked); err != nil {
		return err
	}
	if tmdbChanged && !locked {
		go func() { _ = s.Scanner.RefreshItem(context.Background(), it.ID) }()
	}
	it, err = s.DB.GetItem(it.ID)
	if err != nil {
		return err
	}
	return ok2(s, w, r, it)
}

func (s *Server) refreshItem(w http.ResponseWriter, r *http.Request) error {
	if s.TMDB() == nil {
		return errf(http.StatusBadRequest, "configure a TMDB API key in the settings first")
	}
	id := r.PathValue("id")
	if _, err := s.DB.GetItem(id); err != nil {
		return err
	}
	go func() {
		if err := s.Scanner.RefreshItem(context.Background(), id); err != nil && !errors.Is(err, scanner.ErrBusy) {
			s.Log.Error("refresh", "err", err)
		}
	}()
	return ok(w, map[string]bool{"ok": true})
}
