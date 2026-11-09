package api

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/rubenpari/lumen/internal/db"
)

// ItemDTO is the API representation of an item.
type ItemDTO struct {
	*db.Item
	Images        map[string]string `json:"images"`
	UserData      db.UserData       `json:"userData"`
	SeriesName    string            `json:"seriesName,omitempty"`
	SeasonName    string            `json:"seasonName,omitempty"`
	ChildCount    int               `json:"childCount,omitempty"`
	EpisodeCount  int               `json:"episodeCount,omitempty"`
	UnplayedCount int               `json:"unplayedCount,omitempty"`
	ParentImages  map[string]string `json:"parentImages,omitempty"` // series images for seasons/episodes
	Trailers      []string          `json:"trailers,omitempty"`
}

func imgTag(path string) string {
	if path == "" {
		return ""
	}
	h := sha1.Sum([]byte(path))
	return hex.EncodeToString(h[:6])
}

func imagesOf(it *db.Item) map[string]string {
	m := map[string]string{}
	for k, v := range map[string]string{"primary": it.ImgPrimary, "backdrop": it.ImgBackdrop, "logo": it.ImgLogo, "thumb": it.ImgThumb} {
		if v != "" {
			m[k] = imgTag(v)
		}
	}
	return m
}

// dto converts items into DTOs, attaching user data and aggregated counts.
func (s *Server) dto(r *http.Request, items []*db.Item) ([]*ItemDTO, error) {
	u := userFrom(r)
	facts, err := s.loadItemFacts(u.ID, items)
	if err != nil {
		return nil, err
	}
	out := make([]*ItemDTO, len(items))
	seriesLike := false
	for i, it := range items {
		if !u.IsAdmin {
			it.Path = ""
		}
		if it.Type == db.TypeBook && it.Duration == 0 {
			it.Duration = 100 // book progress is stored as a percentage
		}
		out[i] = &ItemDTO{Item: it, Images: imagesOf(it), UserData: facts.UserData[it.ID]}
		switch it.Type {
		case db.TypeSeries, db.TypeSeason:
			seriesLike = true
		case db.TypeMusicAlbum, db.TypeAudioBook:
			_ = s.DB.QueryRow(`SELECT COUNT(*) FROM items WHERE parent_id=? AND type='Audio'`, it.ID).Scan(&out[i].ChildCount)
		case db.TypeMusicArtist:
			_ = s.DB.QueryRow(`SELECT COUNT(*) FROM items WHERE artist_id=? AND type='MusicAlbum'`, it.ID).Scan(&out[i].ChildCount)
		case db.TypePhotoAlbum:
			_ = s.DB.QueryRow(`SELECT COUNT(*) FROM items WHERE parent_id=? AND type='Photo'`, it.ID).Scan(&out[i].ChildCount)
		}
	}
	if seriesLike {
		for _, d := range out {
			if c, ok := facts.Counts[d.ID]; ok {
				d.EpisodeCount, d.UnplayedCount = c[0], c[0]-c[1]
				d.UserData.Played = c[0] > 0 && c[1] == c[0]
			}
			if d.Type == db.TypeSeries {
				_ = s.DB.QueryRow(`SELECT COUNT(*) FROM items WHERE parent_id=? AND type='Season'`, d.ID).Scan(&d.ChildCount)
			}
		}
	}
	for _, d := range out {
		if p := facts.Parents[d.SeriesID]; p != nil {
			d.SeriesName = p.Name
			d.ParentImages = imagesOf(p)
		}
		if p := facts.Parents[d.SeasonID]; p != nil {
			d.SeasonName = p.Name
			if t := imgTag(p.ImgPrimary); t != "" {
				if d.ParentImages == nil {
					d.ParentImages = map[string]string{}
				}
				d.ParentImages["seasonPrimary"] = t
			}
		}
	}
	return out, nil
}

func (s *Server) one(r *http.Request, it *db.Item) (*ItemDTO, error) {
	d, err := s.dto(r, []*db.Item{it})
	if err != nil {
		return nil, err
	}
	return d[0], nil
}

func (s *Server) listLibraries(w http.ResponseWriter, r *http.Request) error {
	libs, err := s.DB.ListLibraries()
	if err != nil {
		return err
	}
	if libs == nil {
		libs = []*db.Library{}
	}
	type libDTO struct {
		*db.Library
		Images []string `json:"images"` // a few item ids with backdrops for library cards
	}
	out := []libDTO{}
	for _, l := range libs {
		if !userFrom(r).IsAdmin {
			l.Paths = nil
		}
		var ids []string
		rows, err := s.DB.Query(`SELECT id FROM items WHERE library_id=? AND type IN ('Movie','Series') AND extra_type='' AND img_backdrop!='' ORDER BY date_added DESC LIMIT 4`, l.ID)
		if err == nil {
			for rows.Next() {
				var id string
				_ = rows.Scan(&id)
				ids = append(ids, id)
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return err
			}
			rows.Close()
		}
		out = append(out, libDTO{l, ids})
	}
	return ok(w, out)
}

var sortColumns = map[string]string{
	"name":       "i.sort_name COLLATE NOCASE",
	"dateAdded":  "i.date_added",
	"year":       "i.year",
	"premiere":   "i.premiere_date",
	"rating":     "i.community_rating",
	"runtime":    "i.duration",
	"random":     "RANDOM()",
	"lastPlayed": "COALESCE(ud.last_played,0)",
	"episode":    "COALESCE(i.season_number,0), COALESCE(i.episode_number,0), i.sort_name",
	"track":      "COALESCE(i.disc_number,1), COALESCE(i.track_number,0), i.sort_name",
	"taken":      "i.taken_at",
	"artist":     "i.album_artist COLLATE NOCASE",
	"series":     "i.album_artist COLLATE NOCASE, COALESCE(i.track_number,0)",
}

func (s *Server) listItems(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	u := userFrom(r)
	f := listFilter{UserID: u.ID}
	if v := q.Get("libraryId"); v != "" {
		f.LibraryID = v
	}
	if v := q.Get("type"); v != "" {
		f.Types = strings.Split(v, ",")
	} else {
		f.Types = []string{db.TypeMovie, db.TypeSeries}
	}
	if v := q.Get("parentId"); v != "" {
		f.ParentID = v
	}
	if v := q.Get("seriesId"); v != "" {
		f.SeriesIDs = append(f.SeriesIDs, v)
	}
	if v := q.Get("seasonId"); v != "" {
		f.SeasonID = v
	}
	if v := q.Get("albumId"); v != "" {
		f.AlbumID = v
	}
	if v := q.Get("artistId"); v != "" {
		f.ArtistID = v
	}
	if v := q.Get("genre"); v != "" {
		f.Genres = []string{v}
	}
	if v := q.Get("studio"); v != "" {
		f.Studio = v
	}
	if v := q.Get("person"); v != "" {
		f.Person = v
	}
	if v := q.Get("bookSeries"); v != "" {
		f.BookSeries = v
	}
	if v := qint(r, "year", 0); v > 0 {
		f.Year = v
	}
	if v := q.Get("decade"); v != "" {
		var d int
		if _, err := fmt.Sscanf(v, "%d", &d); err == nil {
			f.HasDecade = true
			f.Decade = d
		}
	}
	if v := strings.TrimSpace(q.Get("search")); v != "" {
		f.Search = v
	}
	if v := q.Get("letter"); v != "" {
		f.Letter = v
	}
	switch q.Get("filter") {
	case "favorites":
		f.Favorite = true
	case "unplayed":
		f.SeriesAwareUnplayed = true
	case "played":
		f.RequirePlayed = true
	case "resumable":
		f.Resumable = true
	}
	order := sortColumns[q.Get("sort")]
	if order == "" {
		order = sortColumns["name"]
	}
	dir := "ASC"
	if strings.EqualFold(q.Get("order"), "desc") {
		dir = "DESC"
	}
	if q.Get("root") == "true" {
		f.RootOnly = true
	}
	if q.Get("sort") != "random" && q.Get("sort") != "episode" && q.Get("sort") != "track" {
		order = order + " " + dir + ", i.sort_name COLLATE NOCASE"
	}
	limit := qint(r, "limit", 100)
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	f.Order = order
	f.Limit = limit
	f.Offset = qint(r, "offset", 0)
	items, total, err := s.pageItems(f)
	if err != nil {
		return err
	}
	d, err := s.dto(r, items)
	if err != nil {
		return err
	}
	if d == nil {
		d = []*ItemDTO{}
	}
	return ok(w, map[string]any{"items": d, "total": total})
}

func (s *Server) getItem(w http.ResponseWriter, r *http.Request) error {
	it, err := s.DB.GetItem(r.PathValue("id"))
	if err != nil {
		return err
	}
	d, err := s.one(r, it)
	if err != nil {
		return err
	}
	if it.Type == db.TypeMovie {
		trailers, _ := s.DB.QueryItems(`WHERE parent_id=? AND extra_type='Trailer' ORDER BY name`, it.ID)
		for _, t := range trailers {
			d.Trailers = append(d.Trailers, t.ID)
		}
	}
	return ok(w, d)
}

func (s *Server) children(w http.ResponseWriter, r *http.Request) error {
	it, err := s.DB.GetItem(r.PathValue("id"))
	if err != nil {
		return err
	}
	var kids []*db.Item
	switch it.Type {
	case db.TypeSeries:
		kids, err = s.DB.QueryItems(`WHERE parent_id=? AND type='Season' ORDER BY CASE WHEN season_number=0 THEN 9999 ELSE season_number END`, it.ID)
	case db.TypeSeason:
		kids, err = s.DB.QueryItems(`WHERE season_id=? AND type='Episode' ORDER BY COALESCE(episode_number,9999), premiere_date, sort_name`, it.ID)
	case db.TypeMusicArtist:
		kids, err = s.DB.QueryItems(`WHERE artist_id=? AND type='MusicAlbum' ORDER BY year DESC, sort_name`, it.ID)
	case db.TypeMusicAlbum:
		kids, err = s.DB.QueryItems(`WHERE album_id=? AND type='Audio' ORDER BY COALESCE(disc_number,1), COALESCE(track_number,9999), sort_name`, it.ID)
	case db.TypeAudioBook:
		kids, err = s.DB.QueryItems(`WHERE parent_id=? AND type='Audio' ORDER BY COALESCE(track_number,9999), sort_name`, it.ID)
	case db.TypePhotoAlbum:
		kids, err = s.DB.QueryItems(`WHERE parent_id=? AND type IN ('PhotoAlbum','Photo') ORDER BY CASE type WHEN 'PhotoAlbum' THEN 0 ELSE 1 END, taken_at, sort_name`, it.ID)
	}
	if err != nil {
		return err
	}
	d, err := s.dto(r, kids)
	if err != nil {
		return err
	}
	if d == nil {
		d = []*ItemDTO{}
	}
	return ok(w, d)
}

func (s *Server) similar(w http.ResponseWriter, r *http.Request) error {
	it, err := s.DB.GetItem(r.PathValue("id"))
	if err != nil {
		return err
	}
	items, err := s.similarItems(it)
	if err != nil {
		return err
	}
	d, err := s.dto(r, items)
	if err != nil {
		return err
	}
	if d == nil {
		d = []*ItemDTO{}
	}
	return ok(w, d)
}

// similarItems ranks items of the same library by shared genres, people and year.
func (s *Server) similarItems(it *db.Item) ([]*db.Item, error) {
	if it.Type != db.TypeMovie && it.Type != db.TypeSeries {
		return nil, nil
	}
	cands, err := s.DB.QueryItems(`WHERE library_id=? AND type=? AND id!=? AND extra_type=''`, it.LibraryID, it.Type, it.ID)
	if err != nil {
		return nil, err
	}
	genres := map[string]bool{}
	for _, g := range it.Genres {
		genres[g] = true
	}
	people := map[string]bool{}
	for i, p := range it.People {
		if i < 8 || p.Type == "Director" {
			people[p.Name] = true
		}
	}
	type scored struct {
		it    *db.Item
		score float64
	}
	var sc []scored
	for _, c := range cands {
		score := 0.0
		for _, g := range c.Genres {
			if genres[g] {
				score += 10
			}
		}
		for _, p := range c.People {
			if people[p.Name] {
				score += 4
			}
		}
		if it.Year > 0 && c.Year > 0 {
			diff := it.Year - c.Year
			if diff < 0 {
				diff = -diff
			}
			if diff <= 5 {
				score += float64(5 - diff)
			}
		}
		if score > 0 {
			sc = append(sc, scored{c, score + c.CommunityRating/2})
		}
	}
	sort.Slice(sc, func(i, j int) bool { return sc[i].score > sc[j].score })
	var items []*db.Item
	for i := 0; i < len(sc) && i < 16; i++ {
		items = append(items, sc[i].it)
	}
	return items, nil
}

// orderedEpisodes returns all episodes of a series in watch order (specials last).
func (s *Server) orderedEpisodes(seriesID string) ([]*db.Item, error) {
	return s.DB.QueryItems(`WHERE series_id=? AND type='Episode' ORDER BY CASE WHEN COALESCE(season_number,1)=0 THEN 9999 ELSE COALESCE(season_number,1) END, COALESCE(episode_number,9999), premiere_date, sort_name`, seriesID)
}

func (s *Server) nextEpisode(w http.ResponseWriter, r *http.Request) error {
	it, err := s.DB.GetItem(r.PathValue("id"))
	if err != nil {
		return err
	}
	if it.Type != db.TypeEpisode {
		return ok(w, nil)
	}
	eps, err := s.orderedEpisodes(it.SeriesID)
	if err != nil {
		return err
	}
	for i, e := range eps {
		if e.ID == it.ID && i+1 < len(eps) {
			return ok2(s, w, r, eps[i+1])
		}
	}
	return ok(w, nil)
}

func ok2(s *Server, w http.ResponseWriter, r *http.Request, it *db.Item) error {
	d, err := s.one(r, it)
	if err != nil {
		return err
	}
	return ok(w, d)
}

func (s *Server) genres(w http.ResponseWriter, r *http.Request) error {
	where := "WHERE i.type IN ('Movie','Series') AND i.extra_type=''"
	var args []any
	if v := r.URL.Query().Get("libraryId"); v != "" {
		where += " AND i.library_id=?"
		args = append(args, v)
	}
	rows, err := s.DB.Query(`SELECT g.value, COUNT(*) FROM items i, json_each(i.genres) g `+where+` GROUP BY g.value ORDER BY COUNT(*) DESC`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	type genre struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	out := []genre{}
	for rows.Next() {
		var g genre
		if err := rows.Scan(&g.Name, &g.Count); err != nil {
			return err
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return ok(w, out)
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) error {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		return ok(w, map[string]any{"items": []any{}, "people": []any{}})
	}
	like := "%" + q + "%"
	items, err := s.DB.QueryItems(`WHERE extra_type='' AND type IN ('Movie','Series','Episode','MusicArtist','MusicAlbum','Audio','Book','AudioBook')
		AND (name LIKE ? OR original_title LIKE ? OR (type IN ('Book','AudioBook') AND artists LIKE ?))
		AND NOT (type='Audio' AND parent_id IN (SELECT id FROM items WHERE type='AudioBook'))
		ORDER BY CASE type WHEN 'Movie' THEN 0 WHEN 'Series' THEN 0 WHEN 'MusicArtist' THEN 1 WHEN 'MusicAlbum' THEN 1 WHEN 'Book' THEN 1 WHEN 'AudioBook' THEN 1 ELSE 2 END,
		CASE WHEN name LIKE ? THEN 0 ELSE 1 END, community_rating DESC LIMIT 80`, like, like, like, q+"%")
	if err != nil {
		return err
	}
	d, err := s.dto(r, items)
	if err != nil {
		return err
	}
	if d == nil {
		d = []*ItemDTO{}
	}
	// People
	rows, err := s.DB.Query(`SELECT json_extract(p.value,'$.name'), MAX(COALESCE(json_extract(p.value,'$.image'),'')), COUNT(*) FROM items i, json_each(i.people) p
		WHERE i.type IN ('Movie','Series') AND json_extract(p.value,'$.name') LIKE ? GROUP BY 1 ORDER BY 3 DESC LIMIT 12`, like)
	type person struct {
		Name  string `json:"name"`
		Image string `json:"image,omitempty"`
		Count int    `json:"count"`
	}
	people := []person{}
	if err == nil {
		for rows.Next() {
			var p person
			if rows.Scan(&p.Name, &p.Image, &p.Count) == nil {
				people = append(people, p)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	return ok(w, map[string]any{"items": d, "people": people})
}

func (s *Server) image(w http.ResponseWriter, r *http.Request) error {
	it, err := s.DB.GetItem(r.PathValue("id"))
	if err != nil {
		return err
	}
	var src string
	switch r.PathValue("kind") {
	case "primary":
		src = it.ImgPrimary
	case "backdrop":
		src = it.ImgBackdrop
	case "logo":
		src = it.ImgLogo
	case "thumb":
		src = it.ImgThumb
	}
	if src == "" {
		return errf(http.StatusNotFound, "no image")
	}
	if _, err := os.Stat(src); err != nil {
		return errf(http.StatusNotFound, "image missing")
	}
	out, err := s.Images.Resized(src, min(qint(r, "w", 0), 3840))
	if err != nil {
		return err
	}
	w.Header().Set("Cache-Control", "private, max-age=2592000")
	http.ServeFile(w, r, out)
	return nil
}
