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
	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.ID
	}
	ud, err := s.DB.GetUserData(u.ID, ids)
	if err != nil {
		return nil, err
	}
	out := make([]*ItemDTO, len(items))
	var folderIDs []string
	parentIDs := map[string]bool{}
	for i, it := range items {
		if !u.IsAdmin {
			it.Path = ""
		}
		out[i] = &ItemDTO{Item: it, Images: imagesOf(it), UserData: ud[it.ID]}
		if it.Type == db.TypeSeries || it.Type == db.TypeSeason {
			folderIDs = append(folderIDs, it.ID)
		}
		if it.SeriesID != "" {
			parentIDs[it.SeriesID] = true
		}
		if it.SeasonID != "" {
			parentIDs[it.SeasonID] = true
		}
	}
	if len(folderIDs) > 0 {
		counts, err := s.episodeCounts(u.ID, folderIDs)
		if err != nil {
			return nil, err
		}
		for _, d := range out {
			if c, ok := counts[d.ID]; ok {
				d.EpisodeCount, d.UnplayedCount = c[0], c[0]-c[1]
				d.UserData.Played = c[0] > 0 && c[1] == c[0]
			}
			if d.Type == db.TypeSeries {
				_ = s.DB.QueryRow(`SELECT COUNT(*) FROM items WHERE parent_id=? AND type='Season'`, d.ID).Scan(&d.ChildCount)
			}
		}
	}
	if len(parentIDs) > 0 {
		var pids []any
		var ph []string
		for id := range parentIDs {
			pids = append(pids, id)
			ph = append(ph, "?")
		}
		parents, err := s.DB.QueryItems(`WHERE id IN (`+strings.Join(ph, ",")+`)`, pids...)
		if err != nil {
			return nil, err
		}
		byID := map[string]*db.Item{}
		for _, p := range parents {
			byID[p.ID] = p
		}
		for _, d := range out {
			if p := byID[d.SeriesID]; p != nil {
				d.SeriesName = p.Name
				d.ParentImages = imagesOf(p)
			}
			if p := byID[d.SeasonID]; p != nil {
				d.SeasonName = p.Name
				if t := imgTag(p.ImgPrimary); t != "" {
					if d.ParentImages == nil {
						d.ParentImages = map[string]string{}
					}
					d.ParentImages["seasonPrimary"] = t
				}
			}
		}
	}
	return out, nil
}

// episodeCounts returns [total, played] episode counts for series/season ids.
func (s *Server) episodeCounts(userID string, ids []string) (map[string][2]int, error) {
	out := map[string][2]int{}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := []any{userID}
	for _, id := range ids {
		args = append(args, id)
	}
	args2 := append([]any{userID}, args[1:]...)
	for _, q := range []struct {
		col  string
		args []any
	}{{"series_id", args}, {"season_id", args2}} {
		rows, err := s.DB.Query(`SELECT i.`+q.col+`, COUNT(*), COALESCE(SUM(CASE WHEN ud.played=1 THEN 1 ELSE 0 END),0)
			FROM items i LEFT JOIN user_data ud ON ud.item_id=i.id AND ud.user_id=?
			WHERE i.type='Episode' AND i.`+q.col+` IN (`+ph+`) GROUP BY i.`+q.col, q.args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id string
			var total, played int
			if err := rows.Scan(&id, &total, &played); err != nil {
				rows.Close()
				return nil, err
			}
			out[id] = [2]int{total, played}
		}
		rows.Close()
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
}

func (s *Server) listItems(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	u := userFrom(r)
	where := []string{"i.extra_type=''"}
	args := []any{u.ID}
	if v := q.Get("libraryId"); v != "" {
		where = append(where, "i.library_id=?")
		args = append(args, v)
	}
	if v := q.Get("type"); v != "" {
		types := strings.Split(v, ",")
		where = append(where, "i.type IN ("+strings.TrimSuffix(strings.Repeat("?,", len(types)), ",")+")")
		for _, t := range types {
			args = append(args, t)
		}
	} else {
		where = append(where, "i.type IN ('Movie','Series')")
	}
	for key, col := range map[string]string{"parentId": "i.parent_id", "seriesId": "i.series_id", "seasonId": "i.season_id"} {
		if v := q.Get(key); v != "" {
			where = append(where, col+"=?")
			args = append(args, v)
		}
	}
	if v := q.Get("genre"); v != "" {
		where = append(where, "EXISTS (SELECT 1 FROM json_each(i.genres) WHERE value=?)")
		args = append(args, v)
	}
	if v := q.Get("studio"); v != "" {
		where = append(where, "EXISTS (SELECT 1 FROM json_each(i.studios) WHERE value=?)")
		args = append(args, v)
	}
	if v := q.Get("person"); v != "" {
		where = append(where, "EXISTS (SELECT 1 FROM json_each(i.people) WHERE json_extract(value,'$.name')=?)")
		args = append(args, v)
	}
	if v := qint(r, "year", 0); v > 0 {
		where = append(where, "i.year=?")
		args = append(args, v)
	}
	if v := q.Get("decade"); v != "" {
		var d int
		if _, err := fmt.Sscanf(v, "%d", &d); err == nil {
			where = append(where, "i.year>=? AND i.year<?")
			args = append(args, d, d+10)
		}
	}
	if v := strings.TrimSpace(q.Get("search")); v != "" {
		where = append(where, "(i.name LIKE ? OR i.original_title LIKE ?)")
		args = append(args, "%"+v+"%", "%"+v+"%")
	}
	if v := q.Get("letter"); v != "" {
		if v == "#" {
			where = append(where, "substr(i.sort_name,1,1) NOT GLOB '[A-Za-z]'")
		} else {
			where = append(where, "upper(substr(i.sort_name,1,1))=?")
			args = append(args, strings.ToUpper(v[:1]))
		}
	}
	switch q.Get("filter") {
	case "favorites":
		where = append(where, "ud.favorite=1")
	case "unplayed":
		where = append(where, `(CASE WHEN i.type='Series' THEN EXISTS (SELECT 1 FROM items e LEFT JOIN user_data eu ON eu.item_id=e.id AND eu.user_id=?
			WHERE e.series_id=i.id AND e.type='Episode' AND COALESCE(eu.played,0)=0) ELSE COALESCE(ud.played,0)=0 END)`)
		args = append(args, u.ID)
	case "played":
		where = append(where, "ud.played=1")
	case "resumable":
		where = append(where, "ud.position>0 AND ud.played=0")
	}
	order := sortColumns[q.Get("sort")]
	if order == "" {
		order = sortColumns["name"]
	}
	dir := "ASC"
	if strings.EqualFold(q.Get("order"), "desc") {
		dir = "DESC"
	}
	if q.Get("sort") != "random" && q.Get("sort") != "episode" {
		order = order + " " + dir + ", i.sort_name COLLATE NOCASE"
	}
	limit := qint(r, "limit", 100)
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	offset := qint(r, "offset", 0)
	from := ` FROM items i LEFT JOIN user_data ud ON ud.item_id=i.id AND ud.user_id=? WHERE ` + strings.Join(where, " AND ")
	var total int
	if err := s.DB.QueryRow(`SELECT COUNT(*)`+from, args...).Scan(&total); err != nil {
		return err
	}
	rows, err := s.DB.Query(`SELECT `+db.ItemCols("i")+from+` ORDER BY `+order+fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset), args...)
	if err != nil {
		return err
	}
	var items []*db.Item
	for rows.Next() {
		it, err := db.ScanItem(rows)
		if err != nil {
			rows.Close()
			return err
		}
		items = append(items, it)
	}
	rows.Close()
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
	if it.Type != db.TypeMovie && it.Type != db.TypeSeries {
		return ok(w, []any{})
	}
	cands, err := s.DB.QueryItems(`WHERE library_id=? AND type=? AND id!=? AND extra_type=''`, it.LibraryID, it.Type, it.ID)
	if err != nil {
		return err
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
	d, err := s.dto(r, items)
	if err != nil {
		return err
	}
	if d == nil {
		d = []*ItemDTO{}
	}
	return ok(w, d)
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
	return ok(w, out)
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) error {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		return ok(w, map[string]any{"items": []any{}, "people": []any{}})
	}
	like := "%" + q + "%"
	items, err := s.DB.QueryItems(`WHERE extra_type='' AND type IN ('Movie','Series','Episode') AND (name LIKE ? OR original_title LIKE ?)
		ORDER BY CASE type WHEN 'Movie' THEN 0 WHEN 'Series' THEN 0 ELSE 1 END,
		CASE WHEN name LIKE ? THEN 0 ELSE 1 END, community_rating DESC LIMIT 60`, like, like, q+"%")
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
		rows.Close()
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
