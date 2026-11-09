package livetv

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rubenpari/lumen/internal/db"
)

const playlist = `#EXTM3U
#EXTINF:-1 tvg-id="rai1.it" tvg-name="Rai 1 HD" tvg-logo="http://logo/rai1.png" group-title="Generalisti" tvg-chno="1",Rai 1
http://example.com/rai1.m3u8
#EXTINF:0,84. VOX Schweiz
http://example.com/vox.ts
#EXTINF:-1 tvg-id="bad",Not a channel
not-a-url
#EXTINF:-1 channel-id="news",News 24
#EXTVLCOPT:http-user-agent=VLC/3.0
https://example.com/news
`

func TestParseM3U(t *testing.T) {
	ch := ParseM3U(strings.NewReader(playlist))
	if len(ch) != 3 {
		t.Fatalf("got %d channels: %+v", len(ch), ch)
	}
	if c := ch[0]; c.Name != "Rai 1" || c.Number != "1" || c.TvgID != "rai1.it" || c.Logo != "http://logo/rai1.png" || c.Group != "Generalisti" {
		t.Errorf("rai1: %+v", c)
	}
	if c := ch[1]; c.Name != "VOX Schweiz" || c.Number != "84" {
		t.Errorf("vox: %+v", c)
	}
	if c := ch[2]; c.TvgID != "news" || c.UserAgent != "VLC/3.0" || c.Number != "3" {
		t.Errorf("news: %+v", c)
	}
}

func TestXMLTVTime(t *testing.T) {
	tm, ok := ParseXMLTVTime("20260109203000 +0100")
	if !ok || tm.UTC().Format("2006-01-02 15:04") != "2026-01-09 19:30" {
		t.Fatalf("got %v %v", tm, ok)
	}
}

func TestServiceGuide(t *testing.T) {
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	m3u := filepath.Join(dir, "list.m3u")
	_ = os.WriteFile(m3u, []byte(playlist), 0o644)
	now := time.Now().UTC().Truncate(time.Hour)
	f := func(t time.Time) string { return t.Format("20060102150405 -0700") }
	xmltv := `<?xml version="1.0" encoding="UTF-8"?><tv>
<channel id="rai1.it"><display-name lang="it">Rai 1</display-name></channel>
<channel id="vox.ch"><display-name>VOX Schweiz</display-name></channel>
<programme start="` + f(now.Add(-time.Hour)) + `" stop="` + f(now.Add(time.Hour)) + `" channel="rai1.it"><title lang="en">Morning</title><title lang="it">Mattina</title><desc lang="it">Notizie</desc><category>News</category></programme>
<programme start="` + f(now.Add(time.Hour)) + `" stop="` + f(now.Add(2*time.Hour)) + `" channel="rai1.it"><title>Film</title><episode-num system="onscreen">S1E2</episode-num></programme>
<programme start="` + f(now.Add(-30*time.Minute)) + `" stop="` + f(now.Add(30*time.Minute)) + `" channel="vox.ch"><title>Show</title></programme>
<programme start="` + f(now.Add(-48*time.Hour)) + `" stop="` + f(now.Add(-47*time.Hour)) + `" channel="rai1.it"><title>Old</title></programme>
</tv>`
	guide := filepath.Join(dir, "guide.xml")
	_ = os.WriteFile(guide, []byte(xmltv), 0o644)
	s := New(d, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.Lang = func() string { return "it" }
	ctx := context.Background()
	if _, err := s.AddSource(ctx, Source{Kind: "m3u", Name: "IPTV", URL: m3u}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddSource(ctx, Source{Kind: "xmltv", Name: "EPG", URL: guide}); err != nil {
		t.Fatal(err)
	}
	chans, err := s.Channels()
	if err != nil {
		t.Fatal(err)
	}
	if len(chans) != 3 || chans[0].Name != "Rai 1" || chans[0].Now == nil || chans[0].Now.Title != "Mattina" || chans[0].Next == nil || chans[0].Next.Episode != "S1E2" {
		t.Fatalf("channels: %+v", chans[0])
	}
	// VOX has no tvg-id: matched by display name.
	if chans[2].Name != "VOX Schweiz" || chans[2].Now == nil || chans[2].Now.Title != "Show" {
		t.Fatalf("vox by name: %+v", chans[2])
	}
	if chans[1].UserAgent != "VLC/3.0" || chans[1].URL != "https://example.com/news" {
		t.Fatalf("user agent: %+v", chans[1])
	}
	g, _ := s.Guide(now.Add(-72*time.Hour).Unix(), now.Add(3*time.Hour).Unix())
	if len(g[0].Programs) != 2 {
		t.Fatalf("old programs must be dropped: %+v", g[0].Programs)
	}
	srcs, _ := s.Sources()
	if srcs[0].ItemCount != 3 || srcs[1].ItemCount != 3 {
		t.Fatalf("counts: %+v", srcs)
	}
}
