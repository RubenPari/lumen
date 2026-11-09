package api

import (
	"fmt"
	"strings"

	"github.com/rubenpari/lumen/internal/db"
)

// listFilter is the shared item-list predicate. Each HTTP handler translates
// its own query parameters into this value. API-only rules are fields here
// so there is one SQL builder.
type listFilter struct {
	UserID string

	IDs          []string
	Types        []string
	ExcludeTypes []string
	ForceType    string

	LibraryID        string
	ParentID         string
	SeriesIDs        []string
	SeasonID         string
	AlbumID          string
	AlbumIDs         []string
	ArtistID         string
	ArtistIDGroups   [][]string
	ParentOrSeasonID string

	Genres         []string
	Studio         string
	Person         string
	BookSeries     string
	Year           int
	Years          []string
	HasDecade      bool
	Decade         int
	Search         string
	NameStartsWith string
	NameLessThan   string
	Letter         string

	Favorite            bool
	RequirePlayed       bool
	SimpleUnplayed      bool
	SeriesAwareUnplayed bool
	Resumable           bool
	RootOnly            bool
	NotFolder           bool

	Order  string
	Limit  int
	Offset int
}

func (f listFilter) clauses() (where []string, args []any) {
	args = []any{f.UserID}
	where = []string{"i.extra_type=''"}
	if len(f.IDs) > 0 {
		where = append(where, inClause("i.id", f.IDs, &args))
	}
	if f.LibraryID != "" {
		where = append(where, "i.library_id=?")
		args = append(args, f.LibraryID)
	}
	if len(f.Types) > 0 {
		where = append(where, inClause("i.type", f.Types, &args))
	}
	if f.ForceType != "" {
		where = append(where, "i.type=?")
		args = append(args, f.ForceType)
	}
	if len(f.ExcludeTypes) > 0 {
		where = append(where, "NOT "+inClause("i.type", f.ExcludeTypes, &args))
	}
	if f.ParentID != "" {
		where = append(where, "i.parent_id=?")
		args = append(args, f.ParentID)
	}
	for _, id := range f.SeriesIDs {
		where = append(where, "i.series_id=?")
		args = append(args, id)
	}
	if f.SeasonID != "" {
		where = append(where, "i.season_id=?")
		args = append(args, f.SeasonID)
	}
	if f.AlbumID != "" {
		where = append(where, "i.album_id=?")
		args = append(args, f.AlbumID)
	}
	if len(f.AlbumIDs) > 0 {
		where = append(where, inClause("i.album_id", f.AlbumIDs, &args))
	}
	if f.ArtistID != "" {
		where = append(where, "i.artist_id=?")
		args = append(args, f.ArtistID)
	}
	for _, group := range f.ArtistIDGroups {
		if len(group) > 0 {
			where = append(where, inClause("i.artist_id", group, &args))
		}
	}
	if f.ParentOrSeasonID != "" {
		where = append(where, "(i.parent_id=? OR i.season_id=?)")
		args = append(args, f.ParentOrSeasonID, f.ParentOrSeasonID)
	}
	if f.RootOnly {
		where = append(where, "i.parent_id IS NULL")
	}
	if len(f.Genres) == 1 {
		where = append(where, "EXISTS (SELECT 1 FROM json_each(i.genres) WHERE value=?)")
		args = append(args, f.Genres[0])
	} else if len(f.Genres) > 1 {
		where = append(where, "EXISTS (SELECT 1 FROM json_each(i.genres) WHERE "+inClause("value", f.Genres, &args)+")")
	}
	if f.Studio != "" {
		where = append(where, "EXISTS (SELECT 1 FROM json_each(i.studios) WHERE value=?)")
		args = append(args, f.Studio)
	}
	if f.Person != "" {
		where = append(where, "EXISTS (SELECT 1 FROM json_each(i.people) WHERE json_extract(value,'$.name')=?)")
		args = append(args, f.Person)
	}
	if f.BookSeries != "" {
		where = append(where, "i.album_artist=?")
		args = append(args, f.BookSeries)
	}
	if f.Year > 0 {
		where = append(where, "i.year=?")
		args = append(args, f.Year)
	}
	if len(f.Years) > 0 {
		where = append(where, inClause("i.year", f.Years, &args))
	}
	if f.HasDecade {
		where = append(where, "i.year>=? AND i.year<?")
		args = append(args, f.Decade, f.Decade+10)
	}
	if f.Search != "" {
		where = append(where, "(i.name LIKE ? OR i.original_title LIKE ?)")
		args = append(args, "%"+f.Search+"%", "%"+f.Search+"%")
	}
	if f.NameStartsWith != "" {
		where = append(where, "i.sort_name LIKE ?")
		args = append(args, f.NameStartsWith+"%")
	}
	if f.NameLessThan != "" {
		where = append(where, "i.sort_name < ?")
		args = append(args, f.NameLessThan)
	}
	if f.Letter != "" {
		if f.Letter == "#" {
			where = append(where, "substr(i.sort_name,1,1) NOT GLOB '[A-Za-z]'")
		} else {
			where = append(where, "upper(substr(i.sort_name,1,1))=?")
			args = append(args, strings.ToUpper(f.Letter[:1]))
		}
	}
	if f.Favorite {
		where = append(where, "ud.favorite=1")
	}
	if f.SeriesAwareUnplayed {
		where = append(where, `(CASE WHEN i.type='Series' THEN EXISTS (SELECT 1 FROM items e LEFT JOIN user_data eu ON eu.item_id=e.id AND eu.user_id=?
			WHERE e.series_id=i.id AND e.type='Episode' AND COALESCE(eu.played,0)=0) ELSE COALESCE(ud.played,0)=0 END)`)
		args = append(args, f.UserID)
	}
	if f.SimpleUnplayed {
		where = append(where, "COALESCE(ud.played,0)=0")
	}
	if f.RequirePlayed {
		where = append(where, "ud.played=1")
	}
	if f.Resumable {
		where = append(where, "ud.position>0 AND ud.played=0")
	}
	if f.NotFolder {
		where = append(where, "i.is_folder=0")
	}
	return where, args
}

// pageItems counts matching rows and scans one page of item columns.
func (s *Server) pageItems(f listFilter) ([]*db.Item, int, error) {
	where, args := f.clauses()
	from := ` FROM items i LEFT JOIN user_data ud ON ud.item_id=i.id AND ud.user_id=? WHERE ` + strings.Join(where, " AND ")
	var total int
	if err := s.DB.QueryRow(`SELECT COUNT(*)`+from, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.Query(`SELECT `+db.ItemCols("i")+from+` ORDER BY `+f.Order+fmt.Sprintf(" LIMIT %d OFFSET %d", f.Limit, f.Offset), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var items []*db.Item
	for rows.Next() {
		it, err := db.ScanItem(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, it)
	}
	return items, total, rows.Err()
}

// itemFacts is the user data, parent rows, and episode counts shared by the
// native DTO and the Jellyfin item context.
type itemFacts struct {
	UserData map[string]db.UserData
	Parents  map[string]*db.Item
	Counts   map[string][2]int
}

func (s *Server) loadItemFacts(userID string, items []*db.Item) (*itemFacts, error) {
	ids := make([]string, len(items))
	parentIDs := map[string]bool{}
	var folders []string
	for i, it := range items {
		ids[i] = it.ID
		for _, p := range []string{it.SeriesID, it.SeasonID, it.AlbumID, it.ArtistID} {
			if p != "" {
				parentIDs[p] = true
			}
		}
		if it.Type == db.TypeSeries || it.Type == db.TypeSeason {
			folders = append(folders, it.ID)
		}
	}
	ud, err := s.DB.GetUserData(userID, ids)
	if err != nil {
		return nil, err
	}
	facts := &itemFacts{UserData: ud, Parents: map[string]*db.Item{}}
	if len(parentIDs) > 0 {
		args := make([]any, 0, len(parentIDs))
		for id := range parentIDs {
			args = append(args, id)
		}
		parents, err := s.DB.QueryItems(`WHERE id IN (`+strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")+`)`, args...)
		if err != nil {
			return nil, err
		}
		for _, p := range parents {
			facts.Parents[p.ID] = p
		}
	}
	if len(folders) > 0 {
		facts.Counts, err = s.episodeCounts(userID, folders)
		if err != nil {
			return nil, err
		}
	}
	return facts, nil
}

// episodeCounts returns [total, played] episode counts for series and season ids.
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
