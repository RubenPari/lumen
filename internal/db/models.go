package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

// ErrNotFound is returned when a row does not exist.
var ErrNotFound = errors.New("not found")

// Item types.
const (
	TypeMovie   = "Movie"
	TypeSeries  = "Series"
	TypeSeason  = "Season"
	TypeEpisode = "Episode"

	TypeMusicArtist = "MusicArtist"
	TypeMusicAlbum  = "MusicAlbum"
	TypeAudio       = "Audio"
	TypePhotoAlbum  = "PhotoAlbum"
	TypePhoto       = "Photo"
	TypeBook        = "Book"
	TypeAudioBook   = "AudioBook"
)

// Person is a cast or crew member.
type Person struct {
	Name  string `json:"name"`
	Role  string `json:"role,omitempty"`
	Type  string `json:"type"` // Actor, Director, Writer
	Image string `json:"image,omitempty"`
}

// Stream describes a media stream detected by ffprobe.
type Stream struct {
	Index     int     `json:"index"`
	Type      string  `json:"type"` // video, audio, subtitle
	Codec     string  `json:"codec"`
	Profile   string  `json:"profile,omitempty"`
	Language  string  `json:"language,omitempty"`
	Title     string  `json:"title,omitempty"`
	Channels  int     `json:"channels,omitempty"`
	Width     int     `json:"width,omitempty"`
	Height    int     `json:"height,omitempty"`
	BitRate   int64   `json:"bitRate,omitempty"`
	FrameRate float64 `json:"frameRate,omitempty"`
	PixFmt    string  `json:"pixFmt,omitempty"`
	HDR       bool    `json:"hdr,omitempty"`
	Default   bool    `json:"default,omitempty"`
	Forced    bool    `json:"forced,omitempty"`
}

// MediaInfo is the probed technical information of a file.
type MediaInfo struct {
	Container string   `json:"container,omitempty"`
	BitRate   int64    `json:"bitRate,omitempty"`
	Streams   []Stream `json:"streams,omitempty"`
	Probed    bool     `json:"probed"`
	Chapters  []struct {
		Start float64 `json:"start"`
		Title string  `json:"title"`
	} `json:"chapters,omitempty"`
}

// ExternalSub is a subtitle file next to a video.
type ExternalSub struct {
	Path     string `json:"-"`
	Language string `json:"language,omitempty"`
	Format   string `json:"format"`
	Forced   bool   `json:"forced,omitempty"`
	Default  bool   `json:"default,omitempty"`
	SDH      bool   `json:"sdh,omitempty"`
}

// MarshalJSON keeps the path out of API responses but we still need to persist it.
type externalSubStore struct {
	Path     string `json:"path"`
	Language string `json:"language,omitempty"`
	Format   string `json:"format"`
	Forced   bool   `json:"forced,omitempty"`
	Default  bool   `json:"default,omitempty"`
	SDH      bool   `json:"sdh,omitempty"`
}

// Item is a library entry (movie, series, season or episode).
type Item struct {
	ID              string        `json:"id"`
	LibraryID       string        `json:"libraryId"`
	Type            string        `json:"type"`
	ParentID        string        `json:"parentId,omitempty"`
	SeriesID        string        `json:"seriesId,omitempty"`
	SeasonID        string        `json:"seasonId,omitempty"`
	Name            string        `json:"name"`
	OriginalTitle   string        `json:"originalTitle,omitempty"`
	SortName        string        `json:"sortName,omitempty"`
	Path            string        `json:"path,omitempty"`
	IsFolder        bool          `json:"isFolder"`
	Year            int           `json:"year,omitempty"`
	Overview        string        `json:"overview,omitempty"`
	Tagline         string        `json:"tagline,omitempty"`
	PremiereDate    string        `json:"premiereDate,omitempty"`
	EndDate         string        `json:"endDate,omitempty"`
	Status          string        `json:"status,omitempty"`
	CommunityRating float64       `json:"communityRating,omitempty"`
	OfficialRating  string        `json:"officialRating,omitempty"`
	Duration        float64       `json:"duration,omitempty"`
	SeasonNumber    *int          `json:"seasonNumber,omitempty"`
	EpisodeNumber   *int          `json:"episodeNumber,omitempty"`
	EpisodeEnd      *int          `json:"episodeEnd,omitempty"`
	Container       string        `json:"container,omitempty"`
	Size            int64         `json:"size,omitempty"`
	MTime           int64         `json:"-"`
	Genres          []string      `json:"genres,omitempty"`
	Studios         []string      `json:"studios,omitempty"`
	People          []Person      `json:"people,omitempty"`
	TmdbID          string        `json:"tmdbId,omitempty"`
	ImdbID          string        `json:"imdbId,omitempty"`
	ImgPrimary      string        `json:"-"`
	ImgBackdrop     string        `json:"-"`
	ImgLogo         string        `json:"-"`
	ImgThumb        string        `json:"-"`
	Media           MediaInfo     `json:"media"`
	Subtitles       []ExternalSub `json:"subtitles,omitempty"`
	ExtraType       string        `json:"extraType,omitempty"`
	MetadataLocked  bool          `json:"metadataLocked,omitempty"`
	MetadataFetched int64         `json:"-"`
	DateAdded       int64         `json:"dateAdded"`
	DateUpdated     int64         `json:"dateUpdated"`
	// Music / photos / discs
	AlbumID     string   `json:"albumId,omitempty"`
	ArtistID    string   `json:"artistId,omitempty"`
	Artists     []string `json:"artists,omitempty"`
	AlbumArtist string   `json:"albumArtist,omitempty"`
	TrackNumber *int     `json:"trackNumber,omitempty"`
	DiscNumber  *int     `json:"discNumber,omitempty"`
	TakenAt     int64    `json:"takenAt,omitempty"`
	Width       int      `json:"width,omitempty"`
	Height      int      `json:"height,omitempty"`
	DiscType    string   `json:"discType,omitempty"` // dvd, bluray
}

// BookSeries is the series name for a book. It is stored in AlbumArtist.
func (it *Item) BookSeries() string { return it.AlbumArtist }

// SetBookSeries records a book series name.
func (it *Item) SetBookSeries(name string) { it.AlbumArtist = name }

// BookIndex is the position of a book inside its series. It is stored in TrackNumber.
func (it *Item) BookIndex() *int { return it.TrackNumber }

// SetBookIndex records a book's position inside its series.
func (it *Item) SetBookIndex(n *int) { it.TrackNumber = n }

var itemColumns = []string{"id", "library_id", "type", "parent_id", "series_id", "season_id", "name", "original_title", "sort_name",
	"path", "is_folder", "year", "overview", "tagline", "premiere_date", "end_date", "status", "community_rating", "official_rating", "duration",
	"season_number", "episode_number", "episode_end", "container", "size", "mtime", "genres", "studios", "people", "tmdb_id", "imdb_id",
	"img_primary", "img_backdrop", "img_logo", "img_thumb", "media", "subtitles", "extra_type", "metadata_locked", "metadata_fetched", "date_added", "date_updated",
	"album_id", "artist_id", "artists", "album_artist", "track_number", "disc_number", "taken_at", "width", "height", "disc_type"}

var itemCols = ItemCols("")

// ItemCols returns the select list for items, optionally prefixed by a table alias.
func ItemCols(alias string) string {
	pre := ""
	if alias != "" {
		pre = alias + "."
	}
	parts := make([]string, len(itemColumns))
	for i, c := range itemColumns {
		switch c {
		case "parent_id", "series_id", "season_id", "album_id", "artist_id":
			parts[i] = "COALESCE(" + pre + c + ",'')"
		default:
			parts[i] = pre + c
		}
	}
	return strings.Join(parts, ", ")
}

type scanner interface{ Scan(dest ...any) error }

// ScanItem reads an Item from a row selected with ItemCols.
func ScanItem(r scanner, extra ...any) (*Item, error) {
	var it Item
	var genres, studios, people, media, subs string
	var sn, en, ee, tn, dn sql.NullInt64
	var artists string
	dest := []any{&it.ID, &it.LibraryID, &it.Type, &it.ParentID, &it.SeriesID, &it.SeasonID, &it.Name, &it.OriginalTitle, &it.SortName,
		&it.Path, &it.IsFolder, &it.Year, &it.Overview, &it.Tagline, &it.PremiereDate, &it.EndDate, &it.Status, &it.CommunityRating, &it.OfficialRating, &it.Duration,
		&sn, &en, &ee, &it.Container, &it.Size, &it.MTime, &genres, &studios, &people, &it.TmdbID, &it.ImdbID,
		&it.ImgPrimary, &it.ImgBackdrop, &it.ImgLogo, &it.ImgThumb, &media, &subs, &it.ExtraType, &it.MetadataLocked, &it.MetadataFetched, &it.DateAdded, &it.DateUpdated,
		&it.AlbumID, &it.ArtistID, &artists, &it.AlbumArtist, &tn, &dn, &it.TakenAt, &it.Width, &it.Height, &it.DiscType}
	dest = append(dest, extra...)
	if err := r.Scan(dest...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	toPtr := func(n sql.NullInt64) *int {
		if !n.Valid {
			return nil
		}
		v := int(n.Int64)
		return &v
	}
	it.SeasonNumber, it.EpisodeNumber, it.EpisodeEnd = toPtr(sn), toPtr(en), toPtr(ee)
	it.TrackNumber, it.DiscNumber = toPtr(tn), toPtr(dn)
	_ = json.Unmarshal([]byte(artists), &it.Artists)
	_ = json.Unmarshal([]byte(genres), &it.Genres)
	_ = json.Unmarshal([]byte(studios), &it.Studios)
	_ = json.Unmarshal([]byte(people), &it.People)
	_ = json.Unmarshal([]byte(media), &it.Media)
	var stored []externalSubStore
	_ = json.Unmarshal([]byte(subs), &stored)
	for _, s := range stored {
		it.Subtitles = append(it.Subtitles, ExternalSub(s))
	}
	return &it, nil
}

func js(v any) string {
	b, _ := json.Marshal(v)
	if string(b) == "null" {
		return "[]"
	}
	return string(b)
}

func nullInt(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// UpsertItem inserts or updates an item. On update, metadata fields are only
// overwritten when the item metadata is not locked.
func (d *DB) UpsertItem(it *Item) error {
	if it.SortName == "" {
		it.SortName = SortName(it.Name)
	}
	stored := make([]externalSubStore, len(it.Subtitles))
	for i, s := range it.Subtitles {
		stored[i] = externalSubStore(s)
	}
	media, _ := json.Marshal(it.Media)
	_, err := d.Exec(`INSERT INTO items (`+strings.Join(itemColumns, ",")+`)
	VALUES (`+strings.TrimSuffix(strings.Repeat("?,", len(itemColumns)), ",")+`)
	ON CONFLICT(id) DO UPDATE SET
		parent_id=excluded.parent_id, series_id=excluded.series_id, season_id=excluded.season_id, path=excluded.path,
		is_folder=excluded.is_folder, container=excluded.container, size=excluded.size, mtime=excluded.mtime,
		media=excluded.media, subtitles=excluded.subtitles, extra_type=excluded.extra_type,
		season_number=excluded.season_number, episode_number=excluded.episode_number, episode_end=excluded.episode_end,
		name=CASE WHEN items.metadata_locked=1 THEN items.name ELSE excluded.name END,
		original_title=CASE WHEN items.metadata_locked=1 THEN items.original_title ELSE excluded.original_title END,
		sort_name=CASE WHEN items.metadata_locked=1 THEN items.sort_name ELSE excluded.sort_name END,
		year=CASE WHEN items.metadata_locked=1 THEN items.year ELSE excluded.year END,
		overview=CASE WHEN items.metadata_locked=1 THEN items.overview ELSE excluded.overview END,
		tagline=CASE WHEN items.metadata_locked=1 THEN items.tagline ELSE excluded.tagline END,
		premiere_date=CASE WHEN items.metadata_locked=1 THEN items.premiere_date ELSE excluded.premiere_date END,
		end_date=excluded.end_date, status=excluded.status,
		community_rating=CASE WHEN items.metadata_locked=1 THEN items.community_rating ELSE excluded.community_rating END,
		official_rating=CASE WHEN items.metadata_locked=1 THEN items.official_rating ELSE excluded.official_rating END,
		duration=excluded.duration,
		genres=CASE WHEN items.metadata_locked=1 THEN items.genres ELSE excluded.genres END,
		studios=excluded.studios, people=excluded.people, tmdb_id=excluded.tmdb_id, imdb_id=excluded.imdb_id,
		img_primary=excluded.img_primary, img_backdrop=excluded.img_backdrop, img_logo=excluded.img_logo, img_thumb=excluded.img_thumb,
		metadata_fetched=excluded.metadata_fetched, date_updated=excluded.date_updated,
		album_id=excluded.album_id, artist_id=excluded.artist_id, artists=excluded.artists, album_artist=excluded.album_artist,
		track_number=excluded.track_number, disc_number=excluded.disc_number, taken_at=excluded.taken_at,
		width=excluded.width, height=excluded.height, disc_type=excluded.disc_type`,
		it.ID, it.LibraryID, it.Type, nullStr(it.ParentID), nullStr(it.SeriesID), nullStr(it.SeasonID), it.Name, it.OriginalTitle, it.SortName,
		it.Path, it.IsFolder, it.Year, it.Overview, it.Tagline, it.PremiereDate, it.EndDate, it.Status, it.CommunityRating, it.OfficialRating, it.Duration,
		nullInt(it.SeasonNumber), nullInt(it.EpisodeNumber), nullInt(it.EpisodeEnd), it.Container, it.Size, it.MTime,
		js(it.Genres), js(it.Studios), js(it.People), it.TmdbID, it.ImdbID,
		it.ImgPrimary, it.ImgBackdrop, it.ImgLogo, it.ImgThumb, string(media), js(stored), it.ExtraType, it.MetadataLocked, it.MetadataFetched,
		it.DateAdded, it.DateUpdated,
		nullStr(it.AlbumID), nullStr(it.ArtistID), js(it.Artists), it.AlbumArtist, nullInt(it.TrackNumber), nullInt(it.DiscNumber),
		it.TakenAt, it.Width, it.Height, it.DiscType)
	return err
}

// GetItem returns an item by id.
func (d *DB) GetItem(id string) (*Item, error) {
	return ScanItem(d.QueryRow(`SELECT `+itemCols+` FROM items WHERE id=?`, id))
}

// QueryItems runs a select with the item column list.
func (d *DB) QueryItems(where string, args ...any) ([]*Item, error) {
	rows, err := d.Query(`SELECT `+itemCols+` FROM items `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Item
	for rows.Next() {
		it, err := ScanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// User is an account.
type User struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	PasswordHash string `json:"-"`
	IsAdmin      bool   `json:"isAdmin"`
	HasPassword  bool   `json:"hasPassword"`
	CreatedAt    int64  `json:"createdAt"`
}

func scanUser(r scanner) (*User, error) {
	var u User
	if err := r.Scan(&u.ID, &u.Name, &u.PasswordHash, &u.IsAdmin, &u.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	u.HasPassword = u.PasswordHash != ""
	return &u, nil
}

// GetUser by id.
func (d *DB) GetUser(id string) (*User, error) {
	return scanUser(d.QueryRow(`SELECT id,name,password_hash,is_admin,created_at FROM users WHERE id=?`, id))
}

// GetUserByName by name (case-insensitive).
func (d *DB) GetUserByName(name string) (*User, error) {
	return scanUser(d.QueryRow(`SELECT id,name,password_hash,is_admin,created_at FROM users WHERE name=?`, name))
}

// ListUsers returns all users.
func (d *DB) ListUsers() ([]*User, error) {
	rows, err := d.Query(`SELECT id,name,password_hash,is_admin,created_at FROM users ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// CountUsers returns the number of users.
func (d *DB) CountUsers() int {
	var n int
	_ = d.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n
}

// Library is a collection of folders of a given content type.
type Library struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Type      string   `json:"type"` // movies, tvshows
	Paths     []string `json:"paths"`
	CreatedAt int64    `json:"createdAt"`
	LastScan  int64    `json:"lastScan"`
	ItemCount int      `json:"itemCount"`
	Realtime  bool     `json:"realtime"`
}

// ListLibraries returns all libraries with item counts.
func (d *DB) ListLibraries() ([]*Library, error) {
	rows, err := d.Query(`SELECT l.id,l.name,l.type,l.paths,l.created_at,l.last_scan,l.realtime,
		(SELECT COUNT(*) FROM items i WHERE i.library_id=l.id AND i.type IN ('Movie','Series','MusicAlbum','Photo','Book','AudioBook') AND i.extra_type='')
		FROM libraries l ORDER BY l.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Library
	for rows.Next() {
		var l Library
		var paths string
		if err := rows.Scan(&l.ID, &l.Name, &l.Type, &paths, &l.CreatedAt, &l.LastScan, &l.Realtime, &l.ItemCount); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(paths), &l.Paths)
		out = append(out, &l)
	}
	return out, rows.Err()
}

// GetLibrary by id.
func (d *DB) GetLibrary(id string) (*Library, error) {
	libs, err := d.ListLibraries()
	if err != nil {
		return nil, err
	}
	for _, l := range libs {
		if l.ID == id {
			return l, nil
		}
	}
	return nil, ErrNotFound
}

// SaveLibrary inserts or updates a library.
func (d *DB) SaveLibrary(l *Library) error {
	_, err := d.Exec(`INSERT INTO libraries(id,name,type,paths,created_at,last_scan,realtime) VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, type=excluded.type, paths=excluded.paths, last_scan=excluded.last_scan, realtime=excluded.realtime`,
		l.ID, l.Name, l.Type, js(l.Paths), l.CreatedAt, l.LastScan, l.Realtime)
	return err
}

// UserData is per-user playback state.
type UserData struct {
	Position   float64 `json:"position"`
	Played     bool    `json:"played"`
	PlayCount  int     `json:"playCount"`
	Favorite   bool    `json:"favorite"`
	LastPlayed int64   `json:"lastPlayed,omitempty"`
	// ProgressText stores a reading location (EPUB CFI or page number).
	ProgressText string `json:"progressText,omitempty"`
}

// GetUserData returns user data for many items at once.
func (d *DB) GetUserData(userID string, ids []string) (map[string]UserData, error) {
	out := make(map[string]UserData, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	for start := 0; start < len(ids); start += 500 {
		end := min(start+500, len(ids))
		chunk := ids[start:end]
		args := []any{userID}
		for _, id := range chunk {
			args = append(args, id)
		}
		rows, err := d.Query(`SELECT item_id,position,played,play_count,favorite,last_played,progress_text FROM user_data WHERE user_id=? AND item_id IN (`+
			strings.TrimSuffix(strings.Repeat("?,", len(chunk)), ",")+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id string
			var u UserData
			if err := rows.Scan(&id, &u.Position, &u.Played, &u.PlayCount, &u.Favorite, &u.LastPlayed, &u.ProgressText); err != nil {
				rows.Close()
				return nil, err
			}
			out[id] = u
		}
		rows.Close()
	}
	return out, nil
}

// SaveUserData upserts user data for an item.
func (d *DB) SaveUserData(userID, itemID string, u UserData) error {
	_, err := d.Exec(`INSERT INTO user_data(user_id,item_id,position,played,play_count,favorite,last_played,progress_text) VALUES(?,?,?,?,?,?,?,?)
		ON CONFLICT(user_id,item_id) DO UPDATE SET position=excluded.position, played=excluded.played, play_count=excluded.play_count,
		favorite=excluded.favorite, last_played=excluded.last_played, progress_text=excluded.progress_text`,
		userID, itemID, u.Position, u.Played, u.PlayCount, u.Favorite, u.LastPlayed, u.ProgressText)
	return err
}

var articles = []string{"the ", "a ", "an ", "il ", "lo ", "la ", "i ", "gli ", "le ", "l'", "der ", "die ", "das ", "el ", "los ", "las "}

// SortName strips leading articles (like Jellyfin's SortRemoveWords) and pads
// numbers so that "Movie 10" sorts after "Movie 9".
func SortName(name string) string {
	n := strings.TrimSpace(name)
	lower := strings.ToLower(n)
	for _, a := range articles {
		if strings.HasPrefix(lower, a) && len(n) > len(a) {
			n = strings.TrimSpace(n[len(a):])
			break
		}
	}
	var b strings.Builder
	digits := ""
	flush := func() {
		if digits != "" {
			b.WriteString(strings.Repeat("0", max(0, 6-len(digits))) + digits)
			digits = ""
		}
	}
	for _, r := range n {
		if r >= '0' && r <= '9' {
			digits += string(r)
			continue
		}
		flush()
		b.WriteRune(r)
	}
	flush()
	return b.String()
}
