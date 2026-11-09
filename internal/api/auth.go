package api

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rubenpari/lumen/internal/db"
)

const cookieName = "lumen_token"
const pbkdf2Iter = 210000

// hashPassword uses PBKDF2-SHA512 like Jellyfin's default password hasher.
func hashPassword(pw string) (string, error) {
	if pw == "" {
		return "", nil
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha512.New, pw, salt, pbkdf2Iter, 64)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("$PBKDF2-SHA512$iterations=%d$%s$%s", pbkdf2Iter, hex.EncodeToString(salt), hex.EncodeToString(key)), nil
}

func checkPassword(hash, pw string) bool {
	if hash == "" {
		return pw == ""
	}
	parts := strings.Split(hash, "$")
	if len(parts) != 5 || parts[1] != "PBKDF2-SHA512" {
		return false
	}
	iter, err := strconv.Atoi(strings.TrimPrefix(parts[2], "iterations="))
	if err != nil {
		return false
	}
	salt, err1 := hex.DecodeString(parts[3])
	want, err2 := hex.DecodeString(parts[4])
	if err1 != nil || err2 != nil {
		return false
	}
	got, err := pbkdf2.Key(sha512.New, pw, salt, iter, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

func tokenFrom(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	if c, err := r.Cookie(cookieName); err == nil {
		return c.Value
	}
	return r.URL.Query().Get("token")
}

func (s *Server) userForToken(token string) (*db.User, error) {
	if token == "" {
		return nil, db.ErrNotFound
	}
	uid, last, err := s.DB.LookupSession(token)
	if err != nil {
		return nil, err
	}
	if db.Now()-last > 300 {
		_ = s.DB.TouchSession(token)
	}
	return s.DB.GetUser(uid)
}

func (s *Server) requireUser(h handler) handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		u, err := s.userForToken(tokenFrom(r))
		if err != nil {
			return errf(http.StatusUnauthorized, "authentication required")
		}
		return h(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	}
}

func (s *Server) requireAdmin(h handler) handler {
	return s.requireUser(func(w http.ResponseWriter, r *http.Request) error {
		if !userFrom(r).IsAdmin {
			return errf(http.StatusForbidden, "administrator required")
		}
		return h(w, r)
	})
}

func (s *Server) issueSession(w http.ResponseWriter, r *http.Request, u *db.User) (string, error) {
	token := db.NewID() + db.NewID()
	device := r.UserAgent()
	if len(device) > 200 {
		device = device[:200]
	}
	if err := s.DB.CreateSession(db.Session{Token: token, UserID: u.ID, Device: device}); err != nil {
		return "", err
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: r.TLS != nil, Expires: time.Now().AddDate(1, 0, 0)})
	return token, nil
}

type credentials struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	ServerName string `json:"serverName"`
}

func validName(n string) bool {
	n = strings.TrimSpace(n)
	return n != "" && len(n) <= 64
}

func (s *Server) setup(w http.ResponseWriter, r *http.Request) error {
	if s.DB.CountUsers() > 0 {
		return errf(http.StatusConflict, "setup already completed")
	}
	var c credentials
	if err := decode(r, &c); err != nil {
		return err
	}
	if !validName(c.Username) {
		return errf(http.StatusBadRequest, "invalid username")
	}
	hash, err := hashPassword(c.Password)
	if err != nil {
		return err
	}
	u := &db.User{ID: db.NewID(), Name: strings.TrimSpace(c.Username), PasswordHash: hash, IsAdmin: true, CreatedAt: db.Now()}
	if err := s.DB.CreateUser(u); err != nil {
		return err
	}
	if c.ServerName != "" {
		_ = s.DB.SetSetting("server_name", c.ServerName)
	}
	token, err := s.issueSession(w, r, u)
	if err != nil {
		return err
	}
	u.HasPassword = hash != ""
	return ok(w, map[string]any{"token": token, "user": u})
}

func (s *Server) publicUsers(w http.ResponseWriter, r *http.Request) error {
	if s.DB.GetSetting("hide_users", "false") == "true" {
		return ok(w, []any{})
	}
	users, err := s.DB.ListUsers()
	if err != nil {
		return err
	}
	type pu struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		HasPassword bool   `json:"hasPassword"`
	}
	out := []pu{}
	for _, u := range users {
		out = append(out, pu{u.ID, u.Name, u.HasPassword})
	}
	return ok(w, out)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) error {
	var c credentials
	if err := decode(r, &c); err != nil {
		return err
	}
	u, err := s.DB.GetUserByName(strings.TrimSpace(c.Username))
	if err != nil || !checkPassword(u.PasswordHash, c.Password) {
		time.Sleep(400 * time.Millisecond)
		return errf(http.StatusUnauthorized, "invalid username or password")
	}
	token, err := s.issueSession(w, r, u)
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"token": token, "user": u})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) error {
	_ = s.DB.DeleteSession(tokenFrom(r))
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1})
	s.playing.Delete(userFrom(r).ID)
	return ok(w, map[string]bool{"ok": true})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) error {
	return ok(w, userFrom(r))
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	u := userFrom(r)
	if !checkPassword(u.PasswordHash, body.Current) {
		return errf(http.StatusForbidden, "current password is wrong")
	}
	hash, err := hashPassword(body.New)
	if err != nil {
		return err
	}
	if err := s.DB.SetPassword(u.ID, hash); err != nil {
		return err
	}
	return ok(w, map[string]bool{"ok": true})
}
