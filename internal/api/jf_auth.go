package api

import (
	"context"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/rubenpari/lumen/internal/db"
)

// jfVersion is the Jellyfin server version advertised to clients.
const jfVersion = "10.11.0"

type jfAuthInfo struct {
	Client, Device, DeviceID, Version, Token string
}

var authFieldRe = regexp.MustCompile(`(\w+)="([^"]*)"`)

// parseJFAuth reads the MediaBrowser authorization header and token sources.
func parseJFAuth(r *http.Request) jfAuthInfo {
	var a jfAuthInfo
	for _, h := range []string{r.Header.Get("Authorization"), r.Header.Get("X-Emby-Authorization")} {
		if h == "" {
			continue
		}
		low := strings.ToLower(h)
		if !strings.HasPrefix(low, "mediabrowser") && !strings.HasPrefix(low, "emby") {
			continue
		}
		for _, m := range authFieldRe.FindAllStringSubmatch(h, -1) {
			switch strings.ToLower(m[1]) {
			case "client":
				a.Client = m[2]
			case "device":
				a.Device = m[2]
			case "deviceid":
				a.DeviceID = m[2]
			case "version":
				a.Version = m[2]
			case "token":
				a.Token = m[2]
			}
		}
	}
	if a.Token == "" {
		a.Token = r.Header.Get("X-Emby-Token")
	}
	if a.Token == "" {
		a.Token = r.Header.Get("X-MediaBrowser-Token")
	}
	if a.Token == "" {
		for _, k := range []string{"api_key", "ApiKey"} {
			if v := jfQuery(r, k); v != "" {
				a.Token = v
			}
		}
	}
	if a.Token == "" {
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
			a.Token = strings.TrimPrefix(h, "Bearer ")
		}
	}
	if a.Token == "" {
		if c, err := r.Cookie(cookieName); err == nil {
			a.Token = c.Value
		}
	}
	return a
}

func (s *Server) jfRequireUser(h handler) handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		a := parseJFAuth(r)
		u, err := s.userForToken(a.Token)
		if err != nil {
			return errf(http.StatusUnauthorized, "Unauthorized")
		}
		return h(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	}
}

// serverID returns a stable identifier for this server.
func (s *Server) serverID() string {
	id := s.DB.GetSetting("server_id", "")
	if id == "" {
		id = db.NewID()
		_ = s.DB.SetSetting("server_id", id)
	}
	return id
}

func (s *Server) jfPublicInfo(w http.ResponseWriter, r *http.Request) error {
	host, _ := os.Hostname()
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return jfJSON(w, map[string]any{
		"LocalAddress":           scheme + "://" + r.Host,
		"ServerName":             s.DB.GetSetting("server_name", host),
		"Version":                jfVersion,
		"ProductName":            "Jellyfin Server",
		"OperatingSystem":        runtime.GOOS,
		"Id":                     s.serverID(),
		"StartupWizardCompleted": s.DB.CountUsers() > 0,
	})
}

func (s *Server) jfSystemInfo(w http.ResponseWriter, r *http.Request) error {
	host, _ := os.Hostname()
	return jfJSON(w, map[string]any{
		"LocalAddress":               "http://" + r.Host,
		"ServerName":                 s.DB.GetSetting("server_name", host),
		"Version":                    jfVersion,
		"ProductName":                "Jellyfin Server",
		"OperatingSystem":            runtime.GOOS,
		"OperatingSystemDisplayName": runtime.GOOS,
		"Id":                         s.serverID(),
		"StartupWizardCompleted":     true,
		"HasPendingRestart":          false,
		"IsShuttingDown":             false,
		"SupportsLibraryMonitor":     true,
		"CanSelfRestart":             false,
		"CanLaunchWebBrowser":        false,
		"HasUpdateAvailable":         false,
		"EncoderLocation":            "System",
		"SystemArchitecture":         runtime.GOARCH,
		"WebSocketPortNumber":        0,
		"CompletedInstallations":     []any{},
		"CastReceiverApplications":   []any{},
	})
}

func (s *Server) jfUserDto(u *db.User) map[string]any {
	return map[string]any{
		"Name":                      u.Name,
		"ServerId":                  s.serverID(),
		"Id":                        u.ID,
		"HasPassword":               u.HasPassword,
		"HasConfiguredPassword":     u.HasPassword,
		"HasConfiguredEasyPassword": false,
		"EnableAutoLogin":           false,
		"LastLoginDate":             time.Now().UTC().Format(time.RFC3339),
		"LastActivityDate":          time.Now().UTC().Format(time.RFC3339),
		"Configuration": map[string]any{
			"PlayDefaultAudioTrack":      true,
			"SubtitleLanguagePreference": "",
			"DisplayMissingEpisodes":     false,
			"GroupedFolders":             []string{},
			"SubtitleMode":               "Default",
			"DisplayCollectionsView":     false,
			"EnableLocalPassword":        false,
			"OrderedViews":               []string{},
			"LatestItemsExcludes":        []string{},
			"MyMediaExcludes":            []string{},
			"HidePlayedInLatest":         true,
			"RememberAudioSelections":    true,
			"RememberSubtitleSelections": true,
			"EnableNextEpisodeAutoPlay":  true,
		},
		"Policy": map[string]any{
			"IsAdministrator":                 u.IsAdmin,
			"IsHidden":                        false,
			"IsDisabled":                      false,
			"EnableUserPreferenceAccess":      true,
			"EnableRemoteControlOfOtherUsers": false,
			"EnableSharedDeviceControl":       false,
			"EnableRemoteAccess":              true,
			"EnableLiveTvManagement":          u.IsAdmin,
			"EnableLiveTvAccess":              true,
			"EnableMediaPlayback":             true,
			"EnableAudioPlaybackTranscoding":  true,
			"EnableVideoPlaybackTranscoding":  true,
			"EnablePlaybackRemuxing":          true,
			"EnableContentDeletion":           false,
			"EnableContentDownloading":        true,
			"EnableSyncTranscoding":           false,
			"EnableMediaConversion":           false,
			"EnableAllDevices":                true,
			"EnableAllChannels":               true,
			"EnableAllFolders":                true,
			"EnablePublicSharing":             false,
			"InvalidLoginAttemptCount":        0,
			"LoginAttemptsBeforeLockout":      -1,
			"MaxActiveSessions":               0,
			"RemoteClientBitrateLimit":        0,
			"AuthenticationProviderId":        "Jellyfin.Server.Implementations.Users.DefaultAuthenticationProvider",
			"PasswordResetProviderId":         "Jellyfin.Server.Implementations.Users.DefaultPasswordResetProvider",
			"SyncPlayAccess":                  "None",
			"BlockedTags":                     []string{},
			"AllowedTags":                     []string{},
			"EnabledFolders":                  []string{},
			"EnabledDevices":                  []string{},
			"EnabledChannels":                 []string{},
			"BlockUnratedItems":               []string{},
			"EnableCollectionManagement":      false,
			"EnableSubtitleManagement":        false,
			"EnableLyricManagement":           false,
			"ForceRemoteSourceTranscoding":    false,
		},
	}
}

func (s *Server) jfPublicUsers(w http.ResponseWriter, r *http.Request) error {
	out := []any{}
	if s.DB.GetSetting("hide_users", "false") != "true" {
		users, err := s.DB.ListUsers()
		if err != nil {
			return err
		}
		for _, u := range users {
			out = append(out, s.jfUserDto(u))
		}
	}
	return jfJSON(w, out)
}

func (s *Server) jfAuthenticate(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Username string
		Pw       string
		Password string
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	pw := body.Pw
	if pw == "" {
		pw = body.Password
	}
	u, err := s.DB.GetUserByName(strings.TrimSpace(body.Username))
	if err != nil || !checkPassword(u.PasswordHash, pw) {
		time.Sleep(400 * time.Millisecond)
		return errf(http.StatusUnauthorized, "Invalid username or password entered.")
	}
	a := parseJFAuth(r)
	token := db.NewID() + db.NewID()
	device := a.Device
	if device == "" {
		device = deviceName(r.UserAgent())
	}
	if _, err := s.DB.Exec(`INSERT INTO sessions(token,user_id,device,created_at,last_seen,client,device_id,app_version) VALUES(?,?,?,?,?,?,?,?)`,
		token, u.ID, device, db.Now(), db.Now(), a.Client, a.DeviceID, a.Version); err != nil {
		return err
	}
	return jfJSON(w, map[string]any{
		"User":        s.jfUserDto(u),
		"AccessToken": token,
		"ServerId":    s.serverID(),
		"SessionInfo": s.jfSessionInfo(u, a, device),
	})
}

func (s *Server) jfSessionInfo(u *db.User, a jfAuthInfo, device string) map[string]any {
	return map[string]any{
		"Id":                    db.StableID(u.ID, a.DeviceID),
		"UserId":                u.ID,
		"UserName":              u.Name,
		"Client":                a.Client,
		"DeviceName":            device,
		"DeviceId":              a.DeviceID,
		"ApplicationVersion":    a.Version,
		"ServerId":              s.serverID(),
		"IsActive":              true,
		"SupportsMediaControl":  false,
		"SupportsRemoteControl": false,
		"PlayableMediaTypes":    []string{"Video", "Audio"},
		"SupportedCommands":     []string{},
		"LastActivityDate":      time.Now().UTC().Format(time.RFC3339),
		"PlayState":             map[string]any{"CanSeek": false, "IsPaused": false, "IsMuted": false, "RepeatMode": "RepeatNone"},
		"NowPlayingQueue":       []any{},
		"AdditionalUsers":       []any{},
	}
}

func (s *Server) jfMe(w http.ResponseWriter, r *http.Request) error {
	return jfJSON(w, s.jfUserDto(userFrom(r)))
}

func (s *Server) jfUserByID(w http.ResponseWriter, r *http.Request) error {
	u, err := s.DB.GetUser(normID(jfParam(r, "userId")))
	if err != nil {
		return err
	}
	return jfJSON(w, s.jfUserDto(u))
}

func (s *Server) jfSessions(w http.ResponseWriter, r *http.Request) error {
	u := userFrom(r)
	a := parseJFAuth(r)
	return jfJSON(w, []any{s.jfSessionInfo(u, a, a.Device)})
}

func (s *Server) jfLogout(w http.ResponseWriter, r *http.Request) error {
	_, _ = s.DB.Exec(`DELETE FROM sessions WHERE token=?`, parseJFAuth(r).Token)
	return noContent(w)
}

// normID strips dashes from GUID-formatted ids sent by clients.
func normID(id string) string {
	return strings.ToLower(strings.ReplaceAll(id, "-", ""))
}
