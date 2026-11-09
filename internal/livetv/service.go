package livetv

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/rubenpari/lumen/internal/db"
)

// Source is an M3U tuner or an XMLTV guide.
type Source struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"` // m3u, xmltv
	Name        string `json:"name"`
	URL         string `json:"url"`
	UserAgent   string `json:"userAgent,omitempty"`
	LastRefresh int64  `json:"lastRefresh"`
	LastError   string `json:"lastError,omitempty"`
	ItemCount   int    `json:"itemCount"`
}

// ChannelRow is a channel with its current and next program.
type ChannelRow struct {
	ID        string       `json:"id"`
	SourceID  string       `json:"-"`
	Number    string       `json:"number"`
	Name      string       `json:"name"`
	Logo      string       `json:"logo,omitempty"`
	Group     string       `json:"group,omitempty"`
	TvgID     string       `json:"tvgId,omitempty"`
	URL       string       `json:"-"`
	UserAgent string       `json:"-"`
	Now       *ProgramRow  `json:"now,omitempty"`
	Next      *ProgramRow  `json:"next,omitempty"`
	Programs  []ProgramRow `json:"programs,omitempty"`
}

// ProgramRow is a guide entry.
type ProgramRow struct {
	ID          string `json:"id"`
	ChannelID   string `json:"channelId,omitempty"`
	Start       int64  `json:"start"`
	Stop        int64  `json:"stop"`
	Title       string `json:"title"`
	SubTitle    string `json:"subTitle,omitempty"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Episode     string `json:"episode,omitempty"`
}

// Service manages Live TV sources and data.
type Service struct {
	DB       *db.DB
	Log      *slog.Logger
	Lang     func() string
	OnChange func()
	client   *http.Client
}

// New creates the service.
func New(d *db.DB, log *slog.Logger) *Service {
	return &Service{DB: d, Log: log, Lang: func() string { return "" }, client: &http.Client{Timeout: 2 * time.Minute}}
}

// Sources lists configured sources.
func (s *Service) Sources() ([]Source, error) {
	rows, err := s.DB.Query(`SELECT id,kind,name,url,user_agent,last_refresh,last_error,item_count FROM livetv_sources ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Source{}
	for rows.Next() {
		var src Source
		if err := rows.Scan(&src.ID, &src.Kind, &src.Name, &src.URL, &src.UserAgent, &src.LastRefresh, &src.LastError, &src.ItemCount); err != nil {
			return nil, err
		}
		out = append(out, src)
	}
	return out, rows.Err()
}

// AddSource stores a new source and refreshes it.
func (s *Service) AddSource(ctx context.Context, src Source) (Source, error) {
	src.Kind = strings.ToLower(src.Kind)
	if src.Kind != "m3u" && src.Kind != "xmltv" {
		return src, errors.New("kind must be m3u or xmltv")
	}
	if strings.TrimSpace(src.URL) == "" {
		return src, errors.New("url is required")
	}
	if src.Name == "" {
		src.Name = src.URL
	}
	src.ID = db.NewID()
	if _, err := s.DB.Exec(`INSERT INTO livetv_sources(id,kind,name,url,user_agent,created_at) VALUES(?,?,?,?,?,?)`,
		src.ID, src.Kind, src.Name, strings.TrimSpace(src.URL), src.UserAgent, db.Now()); err != nil {
		return src, err
	}
	err := s.Refresh(ctx, src.ID)
	return src, err
}

// DeleteSource removes a source with its channels/programs.
func (s *Service) DeleteSource(id string) error {
	_, err := s.DB.Exec(`DELETE FROM livetv_sources WHERE id=?`, id)
	if err == nil && s.OnChange != nil {
		s.OnChange()
	}
	return err
}

func (s *Service) open(ctx context.Context, src Source) (io.ReadCloser, error) {
	var rc io.ReadCloser
	if strings.HasPrefix(src.URL, "/") || strings.HasPrefix(src.URL, "file://") {
		f, err := os.Open(strings.TrimPrefix(src.URL, "file://"))
		if err != nil {
			return nil, err
		}
		rc = f
	} else {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.URL, nil)
		if err != nil {
			return nil, err
		}
		ua := src.UserAgent
		if ua == "" {
			ua = "Lumen/1.0"
		}
		req.Header.Set("User-Agent", ua)
		resp, err := s.client.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("%s: %s", src.URL, resp.Status)
		}
		rc = resp.Body
	}
	if strings.HasSuffix(strings.ToLower(src.URL), ".gz") {
		gz, err := gzip.NewReader(rc)
		if err != nil {
			rc.Close()
			return nil, err
		}
		return struct {
			io.Reader
			io.Closer
		}{gz, rc}, nil
	}
	return rc, nil
}

// RefreshAll refreshes every source (M3U first so guides can be matched).
func (s *Service) RefreshAll(ctx context.Context) {
	srcs, err := s.Sources()
	if err != nil {
		return
	}
	sort.SliceStable(srcs, func(i, j int) bool { return srcs[i].Kind == "m3u" && srcs[j].Kind != "m3u" })
	for _, src := range srcs {
		if err := s.Refresh(ctx, src.ID); err != nil {
			s.Log.Warn("livetv refresh", "source", src.Name, "err", err)
		}
	}
}

// Refresh downloads and parses a source.
func (s *Service) Refresh(ctx context.Context, id string) error {
	srcs, err := s.Sources()
	if err != nil {
		return err
	}
	var src *Source
	for i := range srcs {
		if srcs[i].ID == id {
			src = &srcs[i]
		}
	}
	if src == nil {
		return db.ErrNotFound
	}
	count, err := s.refresh(ctx, *src)
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	_, _ = s.DB.Exec(`UPDATE livetv_sources SET last_refresh=?, last_error=?, item_count=CASE WHEN ?='' THEN ? ELSE item_count END WHERE id=?`,
		db.Now(), msg, msg, count, id)
	if s.OnChange != nil {
		s.OnChange()
	}
	return err
}

func (s *Service) refresh(ctx context.Context, src Source) (int, error) {
	rc, err := s.open(ctx, src)
	if err != nil {
		return 0, err
	}
	defer rc.Close()
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	count := 0
	switch src.Kind {
	case "m3u":
		chans := ParseM3U(rc)
		if len(chans) == 0 {
			return 0, errors.New("no channels found in playlist")
		}
		if _, err := tx.Exec(`DELETE FROM livetv_channels WHERE source_id=?`, src.ID); err != nil {
			return 0, err
		}
		for i, c := range chans {
			ua := c.UserAgent
			if ua == "" {
				ua = src.UserAgent
			}
			if _, err := tx.Exec(`INSERT OR REPLACE INTO livetv_channels(id,source_id,number,name,logo,group_title,tvg_id,url,sort) VALUES(?,?,?,?,?,?,?,?,?)`,
				db.StableID("channel", src.ID, c.URL), src.ID, c.Number, c.Name, c.Logo, c.Group, c.TvgID, c.URL+userAgentSuffix(ua), i); err != nil {
				return 0, err
			}
		}
		count = len(chans)
	case "xmltv":
		now := time.Now()
		from, to := now.Add(-6*time.Hour), now.Add(8*24*time.Hour)
		if _, err := tx.Exec(`DELETE FROM livetv_programs WHERE source_id=?`, src.ID); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(`DELETE FROM livetv_guide_channels WHERE source_id=?`, src.ID); err != nil {
			return 0, err
		}
		var perr error
		err := ParseXMLTV(rc, s.Lang(), from, to,
			func(c GuideChannel) {
				_, _ = tx.Exec(`INSERT OR REPLACE INTO livetv_guide_channels(source_id,xml_id,name,icon) VALUES(?,?,?,?)`, src.ID, c.ID, c.Name, c.Icon)
			},
			func(p Program) {
				if perr != nil {
					return
				}
				_, perr = tx.Exec(`INSERT OR REPLACE INTO livetv_programs(id,source_id,channel,start,stop,title,subtitle,description,category,icon,episode) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
					db.StableID("program", src.ID, p.Channel, fmt.Sprint(p.Start.Unix())), src.ID, p.Channel, p.Start.Unix(), p.Stop.Unix(),
					p.Title, p.SubTitle, p.Description, p.Category, p.Icon, p.Episode)
				count++
			})
		if err != nil {
			return 0, err
		}
		if perr != nil {
			return 0, perr
		}
	}
	return count, tx.Commit()
}

// The user agent is stored with the URL ("url<TAB>ua") to keep the schema small.
func userAgentSuffix(ua string) string {
	if ua == "" {
		return ""
	}
	return "\t" + ua
}

func splitURL(stored string) (string, string) {
	if i := strings.IndexByte(stored, '\t'); i > 0 {
		return stored[:i], stored[i+1:]
	}
	return stored, ""
}

// programChannelFilter matches guide entries to a channel by tvg-id or by
// display name when the playlist has no tvg-id.
const programChannelFilter = `(p.channel = c.tvg_id OR (c.tvg_id = '' AND p.channel IN (SELECT xml_id FROM livetv_guide_channels g WHERE lower(g.name)=lower(c.name))))`

func scanChannel(rows interface{ Scan(...any) error }) (ChannelRow, error) {
	var c ChannelRow
	var stored string
	err := rows.Scan(&c.ID, &c.SourceID, &c.Number, &c.Name, &c.Logo, &c.Group, &c.TvgID, &stored)
	c.URL, c.UserAgent = splitURL(stored)
	return c, err
}

const channelCols = `c.id, c.source_id, c.number, c.name, c.logo, c.group_title, c.tvg_id, c.url`

// Channels returns all channels sorted by number with now/next programs.
func (s *Service) Channels() ([]ChannelRow, error) {
	rows, err := s.DB.Query(`SELECT ` + channelCols + ` FROM livetv_channels c ORDER BY CAST(c.number AS REAL), c.sort, c.name`)
	if err != nil {
		return nil, err
	}
	var out []ChannelRow
	for rows.Next() {
		c, err := scanChannel(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, c)
	}
	rows.Close()
	now := time.Now().Unix()
	for i := range out {
		progs, _ := s.programsFor(out[i], now, now+6*3600, 2)
		if len(progs) > 0 && progs[0].Start <= now {
			out[i].Now = &progs[0]
			if len(progs) > 1 {
				out[i].Next = &progs[1]
			}
		} else if len(progs) > 0 {
			out[i].Next = &progs[0]
		}
	}
	if out == nil {
		out = []ChannelRow{}
	}
	return out, nil
}

// Channel returns one channel.
func (s *Service) Channel(id string) (ChannelRow, error) {
	c, err := scanChannel(s.DB.QueryRow(`SELECT `+channelCols+` FROM livetv_channels c WHERE c.id=?`, id))
	if err != nil {
		return c, db.ErrNotFound
	}
	return c, nil
}

func (s *Service) programsFor(c ChannelRow, from, to int64, limit int) ([]ProgramRow, error) {
	q := `SELECT p.id, p.start, p.stop, p.title, p.subtitle, p.description, p.category, p.icon, p.episode
		FROM livetv_programs p, livetv_channels c WHERE c.id=? AND ` + programChannelFilter + ` AND p.stop > ? AND p.start < ? ORDER BY p.start`
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := s.DB.Query(q, c.ID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProgramRow
	for rows.Next() {
		var p ProgramRow
		if err := rows.Scan(&p.ID, &p.Start, &p.Stop, &p.Title, &p.SubTitle, &p.Description, &p.Category, &p.Icon, &p.Episode); err != nil {
			return nil, err
		}
		p.ChannelID = c.ID
		out = append(out, p)
	}
	return out, rows.Err()
}

// Guide returns channels with their programs in [from, to).
func (s *Service) Guide(from, to int64) ([]ChannelRow, error) {
	chans, err := s.Channels()
	if err != nil {
		return nil, err
	}
	for i := range chans {
		progs, err := s.programsFor(chans[i], from, to, 0)
		if err != nil {
			return nil, err
		}
		if progs == nil {
			progs = []ProgramRow{}
		}
		chans[i].Programs = progs
	}
	return chans, nil
}

// Program returns one program by id with its channel.
func (s *Service) Program(id string) (ProgramRow, error) {
	var p ProgramRow
	err := s.DB.QueryRow(`SELECT p.id, c.id, p.start, p.stop, p.title, p.subtitle, p.description, p.category, p.icon, p.episode
		FROM livetv_programs p, livetv_channels c WHERE p.id=? AND `+programChannelFilter+` LIMIT 1`, id).
		Scan(&p.ID, &p.ChannelID, &p.Start, &p.Stop, &p.Title, &p.SubTitle, &p.Description, &p.Category, &p.Icon, &p.Episode)
	if err != nil {
		return p, db.ErrNotFound
	}
	return p, nil
}

// Run refreshes all sources periodically.
func (s *Service) Run(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.RefreshAll(ctx)
		}
	}
}
