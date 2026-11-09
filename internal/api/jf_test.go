package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rubenpari/lumen/internal/db"
	"github.com/rubenpari/lumen/internal/metadata"
	"github.com/rubenpari/lumen/internal/scanner"
	"github.com/rubenpari/lumen/internal/stream"
)

type jfClient struct {
	t     *testing.T
	base  string
	token string
}

func (c *jfClient) do(method, path string, body any, out any) (int, http.Header) {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, r)
	auth := `MediaBrowser Client="Jellyfin Android", Device="Pixel", DeviceId="abc123", Version="2.6.0"`
	if c.token != "" {
		auth += `, Token="` + c.token + `"`
	}
	req.Header.Set("Authorization", auth)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	} else {
		_, _ = io.Copy(io.Discard, resp.Body)
	}
	return resp.StatusCode, resp.Header
}

func TestJellyfinClientFlow(t *testing.T) {
	dir := t.TempDir()
	media := filepath.Join(dir, "media")
	touch(t, filepath.Join(media, "movies", "Inception (2010)", "Inception (2010).mkv"))
	touch(t, filepath.Join(media, "movies", "Inception (2010)", "Inception (2010).it.srt"))
	touch(t, filepath.Join(media, "tv", "Severance", "Season 01", "Severance.S01E01.Good.News.mkv"))
	touch(t, filepath.Join(media, "tv", "Severance", "Season 01", "Severance.S01E02.mkv"))

	database, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ff, _ := exec.LookPath("ffmpeg")
	s := &Server{DB: database, Log: log, CacheDir: dir, FFmpeg: ff, Streams: stream.NewManager(ff, filepath.Join(dir, "tx"), log), Images: stream.NewImageCache(filepath.Join(dir, "img"))}
	s.Scanner = &scanner.Scanner{DB: database, Log: log, TMDB: func() *metadata.TMDB { return nil }}
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	// Create admin + libraries through the Lumen API.
	hash, _ := hashPassword("pw")
	_, _ = database.Exec(`INSERT INTO users(id,name,password_hash,is_admin,created_at) VALUES('u1','admin',?,1,1)`, hash)
	for _, l := range []*db.Library{
		{ID: "lib1", Name: "Film", Type: "movies", Paths: []string{filepath.Join(media, "movies")}, CreatedAt: 1},
		{ID: "lib2", Name: "Serie", Type: "tvshows", Paths: []string{filepath.Join(media, "tv")}, CreatedAt: 2},
	} {
		_ = database.SaveLibrary(l)
	}
	_ = s.Scanner.ScanAll(t.Context())
	for i := 0; i < 200 && (s.Scanner.Status().Running || s.Scanner.Status().Finished == 0); i++ {
		time.Sleep(20 * time.Millisecond)
	}

	c := &jfClient{t: t, base: ts.URL}
	var info map[string]any
	if code, _ := c.do("GET", "/system/info/public", nil, &info); code != 200 || info["ProductName"] != "Jellyfin Server" || info["Version"] == "" {
		t.Fatalf("public info: %d %v", code, info)
	}
	if code, _ := c.do("GET", "/emby/System/Info/Public", nil, nil); code != 200 {
		t.Fatalf("emby prefix: %d", code)
	}
	if code, _ := c.do("GET", "/UserViews", nil, nil); code != 401 {
		t.Fatalf("expected 401 got %d", code)
	}
	var auth struct {
		AccessToken string
		User        struct{ Id string }
		ServerId    string
	}
	if code, _ := c.do("POST", "/Users/AuthenticateByName", map[string]string{"Username": "admin", "Pw": "pw"}, &auth); code != 200 || auth.AccessToken == "" || auth.User.Id != "u1" {
		t.Fatalf("auth: %d %+v", code, auth)
	}
	c.token = auth.AccessToken

	var views struct {
		Items []struct{ Id, Name, CollectionType, Type string }
	}
	c.do("GET", "/UserViews?userId=u1", nil, &views)
	if len(views.Items) != 2 || views.Items[0].CollectionType != "movies" || views.Items[1].Type != "CollectionFolder" {
		t.Fatalf("views: %+v", views)
	}
	type qr struct {
		Items []map[string]any
		TotalRecordCount int
	}
	var movies qr
	c.do("GET", "/Users/u1/Items?ParentId=lib1&Recursive=true&IncludeItemTypes=Movie&SortBy=SortName&SortOrder=Ascending&Fields=MediaSources", nil, &movies)
	if movies.TotalRecordCount != 1 || movies.Items[0]["Name"] != "Inception" || movies.Items[0]["ProductionYear"].(float64) != 2010 {
		t.Fatalf("movies: %+v", movies)
	}
	movieID := movies.Items[0]["Id"].(string)
	var shows qr
	c.do("GET", "/Items?ParentId=lib2", nil, &shows)
	if len(shows.Items) != 1 || shows.Items[0]["Type"] != "Series" {
		t.Fatalf("shows: %+v", shows)
	}
	seriesID := shows.Items[0]["Id"].(string)
	var seasons, eps qr
	c.do("GET", "/Shows/"+seriesID+"/Seasons?userId=u1", nil, &seasons)
	c.do("GET", "/Shows/"+seriesID+"/Episodes?SeasonId="+seasons.Items[0]["Id"].(string), nil, &eps)
	if len(eps.Items) != 2 || eps.Items[0]["IndexNumber"].(float64) != 1 || eps.Items[0]["SeriesName"] != "Severance" {
		t.Fatalf("episodes: %+v", eps)
	}
	// Dashed GUIDs are accepted and literal routes win over {itemId}.
	dashed := movieID[:8] + "-" + movieID[8:12] + "-" + movieID[12:16] + "-" + movieID[16:20] + "-" + movieID[20:]
	var one map[string]any
	if code, _ := c.do("GET", "/Items/"+strings.ToUpper(dashed), nil, &one); code != 200 || one["Id"] != movieID {
		t.Fatalf("item by guid: %d %v", code, one["Id"])
	}
	var latest []map[string]any
	c.do("GET", "/Items/Latest?ParentId=lib2", nil, &latest)
	if len(latest) != 1 || latest[0]["Type"] != "Series" {
		t.Fatalf("latest: %+v", latest)
	}

	// PlaybackInfo: a profile that cannot play MKV gets a transcoding URL.
	profile := map[string]any{"DeviceProfile": map[string]any{"DirectPlayProfiles": []map[string]string{{"Container": "mp4", "VideoCodec": "h264", "AudioCodec": "aac", "Type": "Video"}}}}
	var pi struct {
		MediaSources  []map[string]any
		PlaySessionId string
	}
	c.do("POST", "/Items/"+movieID+"/PlaybackInfo?UserId=u1", profile, &pi)
	src := pi.MediaSources[0]
	if src["SupportsDirectPlay"] != false || pi.PlaySessionId == "" {
		t.Fatalf("playback info: %+v", src)
	}
	if ff != "" && !strings.Contains(src["TranscodingUrl"].(string), "master.m3u8") {
		t.Fatalf("missing transcoding url: %+v", src["TranscodingUrl"])
	}
	streams := src["MediaStreams"].([]any)
	ext := streams[len(streams)-1].(map[string]any)
	if ext["IsExternal"] != true || ext["Language"] != "ita" || !strings.Contains(ext["DeliveryUrl"].(string), "/Subtitles/") {
		t.Fatalf("external subtitle: %+v", ext)
	}
	c.do("POST", "/Items/"+movieID+"/PlaybackInfo", map[string]any{}, &pi)
	if pi.MediaSources[0]["SupportsDirectPlay"] != true {
		t.Fatal("no profile should allow direct play")
	}
	if code, h := c.do("GET", "/Videos/"+movieID+"/stream?static=true&api_key="+auth.AccessToken, nil, nil); code != 200 || h.Get("Content-Type") == "" {
		t.Fatalf("stream: %d", code)
	}

	// Watch episode 1 -> NextUp returns episode 2; resume works.
	ep1, ep2 := eps.Items[0]["Id"].(string), eps.Items[1]["Id"].(string)
	_, _ = database.Exec(`UPDATE items SET duration=3000 WHERE type IN ('Episode','Movie')`)
	c.do("POST", "/Sessions/Playing", map[string]any{"ItemId": ep1, "PositionTicks": 0}, nil)
	if code, _ := c.do("POST", "/Sessions/Playing/Stopped", map[string]any{"ItemId": ep1, "PositionTicks": int64(2950) * ticksPerSecond}, nil); code != 204 {
		t.Fatalf("stopped: %d", code)
	}
	var next qr
	c.do("GET", "/Shows/NextUp?UserId=u1", nil, &next)
	if len(next.Items) != 1 || next.Items[0]["Id"] != ep2 {
		t.Fatalf("next up: %+v", next)
	}
	c.do("POST", "/Sessions/Playing/Progress", map[string]any{"ItemId": movieID, "PositionTicks": int64(600) * ticksPerSecond}, nil)
	var resume qr
	c.do("GET", "/UserItems/Resume?userId=u1", nil, &resume)
	ud := resume.Items[0]["UserData"].(map[string]any)
	if len(resume.Items) != 1 || ud["PlaybackPositionTicks"].(float64) != float64(600*ticksPerSecond) {
		t.Fatalf("resume: %+v", resume)
	}
	var fav map[string]any
	c.do("POST", "/UserFavoriteItems/"+movieID, nil, &fav)
	if fav["IsFavorite"] != true {
		t.Fatalf("favorite: %+v", fav)
	}
	var played map[string]any
	c.do("POST", "/Users/u1/PlayedItems/"+seriesID, nil, &played)
	var show map[string]any
	c.do("GET", "/Items/"+seriesID, nil, &show)
	if show["UserData"].(map[string]any)["Played"] != true {
		t.Fatalf("series played: %+v", show["UserData"])
	}
	// Web client routes still work.
	if code, _ := c.do("GET", "/api/system/info", nil, nil); code != 200 {
		t.Fatalf("lumen api: %d", code)
	}
}
