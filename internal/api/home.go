package api

import (
	"net/http"

	"github.com/rubenpari/lumen/internal/db"
)

type homeRow struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Kind      string     `json:"kind"` // resume, nextup, latest, favorites
	LibraryID string     `json:"libraryId,omitempty"`
	Items     []*ItemDTO `json:"items"`
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) error {
	u := userFrom(r)
	var rows []homeRow

	add := func(row homeRow, items []*db.Item) error {
		if len(items) == 0 {
			return nil
		}
		d, err := s.dto(r, items)
		if err != nil {
			return err
		}
		row.Items = d
		rows = append(rows, row)
		return nil
	}

	resume, err := s.DB.QueryItems(`JOIN user_data ud ON ud.item_id=items.id AND ud.user_id=?
		WHERE items.type IN ('Movie','Episode') AND ud.position>0 AND ud.played=0 ORDER BY ud.last_played DESC LIMIT 20`, u.ID)
	if err != nil {
		return err
	}
	if err := add(homeRow{ID: "resume", Title: "Continue watching", Kind: "resume"}, resume); err != nil {
		return err
	}

	next, err := s.nextUp(u.ID, 20)
	if err != nil {
		return err
	}
	if err := add(homeRow{ID: "nextup", Title: "Next up", Kind: "nextup"}, next); err != nil {
		return err
	}

	libs, err := s.DB.ListLibraries()
	if err != nil {
		return err
	}
	for _, l := range libs {
		var items []*db.Item
		if l.Type == "tvshows" {
			items, err = s.DB.QueryItems(`WHERE library_id=? AND type='Series' ORDER BY date_added DESC LIMIT 24`, l.ID)
		} else {
			items, err = s.DB.QueryItems(`WHERE library_id=? AND type='Movie' AND extra_type='' ORDER BY date_added DESC LIMIT 24`, l.ID)
		}
		if err != nil {
			return err
		}
		if err := add(homeRow{ID: "latest-" + l.ID, Title: "Recently added in " + l.Name, Kind: "latest", LibraryID: l.ID}, items); err != nil {
			return err
		}
	}

	favs, err := s.DB.QueryItems(`JOIN user_data ud ON ud.item_id=items.id AND ud.user_id=?
		WHERE items.type IN ('Movie','Series') AND ud.favorite=1 ORDER BY items.sort_name LIMIT 30`, u.ID)
	if err != nil {
		return err
	}
	if err := add(homeRow{ID: "favorites", Title: "Your favorites", Kind: "favorites"}, favs); err != nil {
		return err
	}

	// Featured: recent, well-rated items with a backdrop, shuffled.
	featured, err := s.DB.QueryItems(`WHERE id IN (SELECT id FROM items WHERE type IN ('Movie','Series') AND extra_type='' AND img_backdrop!=''
		ORDER BY date_added DESC LIMIT 40) ORDER BY RANDOM() LIMIT 6`)
	if err != nil {
		return err
	}
	fd, err := s.dto(r, featured)
	if err != nil {
		return err
	}
	if fd == nil {
		fd = []*ItemDTO{}
	}
	if rows == nil {
		rows = []homeRow{}
	}
	return ok(w, map[string]any{"featured": fd, "rows": rows})
}

// nextUp computes, for recently watched series, the next episode to watch
// (port of Jellyfin's TVSeriesManager.GetNextUp).
func (s *Server) nextUp(userID string, limit int) ([]*db.Item, error) {
	rows, err := s.DB.Query(`SELECT i.series_id, MAX(ud.last_played) lp FROM user_data ud JOIN items i ON i.id=ud.item_id
		WHERE ud.user_id=? AND i.type='Episode' AND (ud.played=1 OR ud.position>0) AND ud.last_played>0
		GROUP BY i.series_id ORDER BY lp DESC LIMIT ?`, userID, limit*2)
	if err != nil {
		return nil, err
	}
	var series []string
	for rows.Next() {
		var id string
		var lp int64
		if err := rows.Scan(&id, &lp); err != nil {
			rows.Close()
			return nil, err
		}
		series = append(series, id)
	}
	rows.Close()
	var out []*db.Item
	for _, sid := range series {
		eps, err := s.orderedEpisodes(sid)
		if err != nil {
			return nil, err
		}
		ids := make([]string, len(eps))
		for i, e := range eps {
			ids[i] = e.ID
		}
		ud, err := s.DB.GetUserData(userID, ids)
		if err != nil {
			return nil, err
		}
		// Most recently played episode (excluding specials).
		lastIdx, lastTime := -1, int64(0)
		for i, e := range eps {
			if e.SeasonNumber != nil && *e.SeasonNumber == 0 {
				continue
			}
			if d := ud[e.ID]; d.Played && d.LastPlayed >= lastTime {
				lastIdx, lastTime = i, d.LastPlayed
			}
		}
		if lastIdx < 0 {
			continue
		}
		for i := lastIdx + 1; i < len(eps); i++ {
			e := eps[i]
			if e.SeasonNumber != nil && *e.SeasonNumber == 0 {
				continue
			}
			d := ud[e.ID]
			if d.Played {
				continue
			}
			if d.Position > 0 {
				break // already in "continue watching"
			}
			out = append(out, e)
			break
		}
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}
