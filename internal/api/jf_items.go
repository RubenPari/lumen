package api

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rubenpari/lumen/internal/db"
)

const ticksPerSecond = 10_000_000

func toTicks(sec float64) int64 { return int64(sec * ticksPerSecond) }

func fromTicks(t int64) float64 { return float64(t) / ticksPerSecond }

// jfType maps Lumen item types to Jellyfin BaseItemKind values.
func jfType(t string) string {
	switch t {
	case db.TypeAudioBook:
		return "Folder"
	}
	return t
}

func jfMediaType(it *db.Item) string {
	switch it.Type {
	case db.TypeMovie, db.TypeEpisode:
		return "Video"
	case db.TypeAudio:
		return "Audio"
	case db.TypePhoto:
		return "Photo"
	case db.TypeBook:
		return "Book"
	}
	return "Unknown"
}

var collectionTypes = map[string]string{"movies": "movies", "tvshows": "tvshows", "music": "music", "photos": "photos", "books": "books"}

func isoDate(s string) any {
	if s == "" {
		return nil
	}
	if len(s) == 10 {
		return s + "T00:00:00.0000000Z"
	}
	return s
}

func unixISO(t int64) string { return time.Unix(t, 0).UTC().Format("2006-01-02T15:04:05.0000000Z") }

type jfCtx struct {
	ud       map[string]db.UserData
	parents  map[string]*db.Item
	counts   map[string][2]int
	sources  bool
	serverID string
}

func (s *Server) jfContext(r *http.Request, items []*db.Item, withSources bool) (*jfCtx, error) {
	u := userFrom(r)
	ids := make([]string, 0, len(items))
	parentIDs := map[string]bool{}
	var folders []string
	for _, it := range items {
		ids = append(ids, it.ID)
		for _, p := range []string{it.SeriesID, it.SeasonID, it.AlbumID, it.ArtistID} {
			if p != "" {
				parentIDs[p] = true
			}
		}
		if it.Type == db.TypeSeries || it.Type == db.TypeSeason {
			folders = append(folders, it.ID)
		}
	}
	ud, err := s.DB.GetUserData(u.ID, ids)
	if err != nil {
		return nil, err
	}
	c := &jfCtx{ud: ud, parents: map[string]*db.Item{}, sources: withSources, serverID: s.serverID()}
	if len(parentIDs) > 0 {
		var args []any
		for id := range parentIDs {
			args = append(args, id)
		}
		ps, err := s.DB.QueryItems(`WHERE id IN (`+strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")+`)`, args...)
		if err != nil {
			return nil, err
		}
		for _, p := range ps {
			c.parents[p.ID] = p
		}
	}
	if len(folders) > 0 {
		if c.counts, err = s.episodeCounts(u.ID, folders); err != nil {
			return nil, err
		}
	}
	return c, nil
}

func jfUserData(it *db.Item, d db.UserData, counts [2]int, hasCounts bool) map[string]any {
	out := map[string]any{
		"PlaybackPositionTicks": toTicks(d.Position),
		"PlayCount":             d.PlayCount,
		"IsFavorite":            d.Favorite,
		"Played":                d.Played,
		"Key":                   it.ID,
		"ItemId":                it.ID,
	}
	if d.LastPlayed > 0 {
		out["LastPlayedDate"] = unixISO(d.LastPlayed)
	}
	if it.Duration > 0 && d.Position > 0 {
		out["PlayedPercentage"] = d.Position / it.Duration * 100
	}
	if hasCounts {
		out["UnplayedItemCount"] = counts[0] - counts[1]
		out["Played"] = counts[0] > 0 && counts[0] == counts[1]
	}
	return out
}

// jfItem converts an item to a BaseItemDto.
func (s *Server) jfItem(c *jfCtx, it *db.Item) map[string]any {
	d := map[string]any{
		"Name":         it.Name,
		"ServerId":     c.serverID,
		"Id":           it.ID,
		"Etag":         imgTag(fmt.Sprint(it.DateUpdated)),
		"DateCreated":  unixISO(it.DateAdded),
		"CanDelete":    false,
		"CanDownload":  !it.IsFolder,
		"SortName":     it.SortName,
		"Type":         jfType(it.Type),
		"IsFolder":     it.IsFolder || it.Type == db.TypeAudioBook,
		"LocationType": "FileSystem",
		"MediaType":    jfMediaType(it),
		"ChannelId":    nil,
	}
	if it.OriginalTitle != "" {
		d["OriginalTitle"] = it.OriginalTitle
	}
	if it.Overview != "" {
		d["Overview"] = it.Overview
	}
	if it.Tagline != "" {
		d["Taglines"] = []string{it.Tagline}
	} else {
		d["Taglines"] = []string{}
	}
	if pd := isoDate(it.PremiereDate); pd != nil {
		d["PremiereDate"] = pd
	}
	if it.EndDate != "" {
		d["EndDate"] = isoDate(it.EndDate)
	}
	if it.Year > 0 {
		d["ProductionYear"] = it.Year
	}
	if it.CommunityRating > 0 {
		d["CommunityRating"] = it.CommunityRating
	}
	if it.OfficialRating != "" {
		d["OfficialRating"] = it.OfficialRating
	}
	if it.Duration > 0 {
		d["RunTimeTicks"] = toTicks(it.Duration)
	}
	genres := it.Genres
	if genres == nil {
		genres = []string{}
	}
	d["Genres"] = genres
	gi := make([]map[string]string, 0, len(genres))
	for _, g := range genres {
		gi = append(gi, map[string]string{"Name": g, "Id": db.StableID("genre", g)})
	}
	d["GenreItems"] = gi
	studios := make([]map[string]string, 0, len(it.Studios))
	for _, st := range it.Studios {
		studios = append(studios, map[string]string{"Name": st, "Id": db.StableID("studio", st)})
	}
	d["Studios"] = studios
	people := make([]map[string]any, 0, len(it.People))
	for _, p := range it.People {
		people = append(people, map[string]any{"Name": p.Name, "Id": db.StableID("person", p.Name), "Role": p.Role, "Type": p.Type})
	}
	d["People"] = people
	prov := map[string]string{}
	if it.TmdbID != "" {
		prov["Tmdb"] = it.TmdbID
	}
	if it.ImdbID != "" {
		prov["Imdb"] = it.ImdbID
	}
	d["ProviderIds"] = prov
	if it.Status != "" {
		d["Status"] = it.Status
	}
	if it.Container != "" && !it.IsFolder {
		d["Container"] = it.Container
	}

	// Hierarchy
	switch it.Type {
	case db.TypeEpisode, db.TypeSeason:
		d["ParentId"] = it.ParentID
		if sr := c.parents[it.SeriesID]; sr != nil {
			d["SeriesId"], d["SeriesName"] = sr.ID, sr.Name
			if sr.ImgPrimary != "" {
				d["SeriesPrimaryImageTag"] = imgTag(sr.ImgPrimary)
			}
			if sr.ImgBackdrop != "" {
				d["ParentBackdropItemId"], d["ParentBackdropImageTags"] = sr.ID, []string{imgTag(sr.ImgBackdrop)}
			}
			if sr.ImgLogo != "" {
				d["ParentLogoItemId"], d["ParentLogoImageTag"] = sr.ID, imgTag(sr.ImgLogo)
			}
			if sr.ImgThumb != "" {
				d["ParentThumbItemId"], d["ParentThumbImageTag"] = sr.ID, imgTag(sr.ImgThumb)
			} else if sr.ImgBackdrop != "" {
				d["ParentThumbItemId"], d["ParentThumbImageTag"] = sr.ID, imgTag(sr.ImgBackdrop)
			}
		}
		if it.Type == db.TypeEpisode {
			if se := c.parents[it.SeasonID]; se != nil {
				d["SeasonId"], d["SeasonName"] = se.ID, se.Name
				d["ParentIndexNumber"] = se.SeasonNumber
			}
			if it.SeasonNumber != nil {
				d["ParentIndexNumber"] = *it.SeasonNumber
			}
			if it.EpisodeNumber != nil {
				d["IndexNumber"] = *it.EpisodeNumber
			}
			if it.EpisodeEnd != nil {
				d["IndexNumberEnd"] = *it.EpisodeEnd
			}
		} else {
			d["IndexNumber"] = it.SeasonNumber
			d["SeriesId"] = it.SeriesID
		}
	case db.TypeAudio:
		d["ParentId"] = it.ParentID
		if al := c.parents[it.AlbumID]; al != nil {
			d["Album"], d["AlbumId"] = al.Name, al.ID
			if al.ImgPrimary != "" {
				d["AlbumPrimaryImageTag"] = imgTag(al.ImgPrimary)
			}
		}
		if it.TrackNumber != nil {
			d["IndexNumber"] = *it.TrackNumber
		}
		if it.DiscNumber != nil {
			d["ParentIndexNumber"] = *it.DiscNumber
		}
	case db.TypeMusicAlbum:
		d["ParentId"] = it.ArtistID
	case db.TypePhoto, db.TypePhotoAlbum:
		if it.ParentID != "" {
			d["ParentId"] = it.ParentID
		}
		if it.Width > 0 {
			d["Width"], d["Height"] = it.Width, it.Height
		}
	}
	if it.Type == db.TypeAudio || it.Type == db.TypeMusicAlbum || it.Type == db.TypeBook {
		artists := it.Artists
		if artists == nil {
			artists = []string{}
		}
		d["Artists"] = artists
		ai := make([]map[string]string, 0, len(artists))
		for _, a := range artists {
			ai = append(ai, map[string]string{"Name": a, "Id": db.StableID(it.LibraryID, db.TypeMusicArtist, strings.ToLower(a))})
		}
		d["ArtistItems"] = ai
		if it.AlbumArtist != "" {
			d["AlbumArtist"] = it.AlbumArtist
			d["AlbumArtists"] = []map[string]string{{"Name": it.AlbumArtist, "Id": it.ArtistID}}
		}
	}

	// Images
	tags := map[string]string{}
	primary := it.ImgPrimary
	if it.Type == db.TypeEpisode && it.ImgThumb != "" {
		primary = it.ImgThumb
	}
	if it.Type == db.TypeMusicArtist && primary == "" {
		primary = it.ImgThumb
	}
	if primary != "" {
		tags["Primary"] = imgTag(primary)
	}
	if it.ImgLogo != "" {
		tags["Logo"] = imgTag(it.ImgLogo)
	}
	if it.ImgThumb != "" && it.Type != db.TypeEpisode {
		tags["Thumb"] = imgTag(it.ImgThumb)
	}
	d["ImageTags"] = tags
	if it.ImgBackdrop != "" {
		d["BackdropImageTags"] = []string{imgTag(it.ImgBackdrop)}
	} else {
		d["BackdropImageTags"] = []string{}
	}
	switch it.Type {
	case db.TypeMovie, db.TypeSeries, db.TypeBook:
		d["PrimaryImageAspectRatio"] = 2.0 / 3.0
	case db.TypeEpisode:
		d["PrimaryImageAspectRatio"] = 16.0 / 9.0
	case db.TypeMusicAlbum, db.TypeMusicArtist, db.TypeAudio, db.TypeAudioBook:
		d["PrimaryImageAspectRatio"] = 1.0
	}

	// User data & counts
	counts, hasCounts := c.counts[it.ID]
	d["UserData"] = jfUserData(it, c.ud[it.ID], counts, hasCounts)
	if hasCounts {
		d["RecursiveItemCount"] = counts[0]
	}
	if it.Type == db.TypeMovie || it.Type == db.TypeEpisode {
		d["VideoType"] = "VideoFile"
		d["HasSubtitles"] = len(it.Subtitles) > 0 || hasSubStream(it)
		d["Chapters"] = jfChapters(it)
		if c.sources {
			src := s.jfMediaSource(it)
			d["MediaSources"] = []any{src}
			d["MediaStreams"] = src["MediaStreams"]
		}
	}
	if it.Type == db.TypeAudio && c.sources {
		src := s.jfMediaSource(it)
		d["MediaSources"] = []any{src}
		d["MediaStreams"] = src["MediaStreams"]
	}
	return d
}

func hasSubStream(it *db.Item) bool {
	for _, st := range it.Media.Streams {
		if st.Type == "subtitle" {
			return true
		}
	}
	return false
}

func jfChapters(it *db.Item) []map[string]any {
	out := []map[string]any{}
	for _, ch := range it.Media.Chapters {
		out = append(out, map[string]any{"StartPositionTicks": toTicks(ch.Start), "Name": ch.Title})
	}
	return out
}

func (s *Server) jfItems(r *http.Request, items []*db.Item) ([]map[string]any, error) {
	fields := strings.ToLower(jfQuery(r, "Fields"))
	withSources := strings.Contains(fields, "mediasources") || strings.Contains(fields, "mediastreams")
	c, err := s.jfContext(r, items, withSources)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		out = append(out, s.jfItem(c, it))
	}
	return out, nil
}

func (s *Server) jfQueryResult(w http.ResponseWriter, r *http.Request, items []*db.Item, total, start int) error {
	dtos, err := s.jfItems(r, items)
	if err != nil {
		return err
	}
	return jfJSON(w, map[string]any{"Items": dtos, "TotalRecordCount": total, "StartIndex": start})
}

// ---- Views ----

func (s *Server) jfLibraryDto(l *db.Library) map[string]any {
	d := map[string]any{
		"Name": l.Name, "ServerId": s.serverID(), "Id": l.ID, "Etag": imgTag(l.ID), "DateCreated": unixISO(l.CreatedAt),
		"CanDelete": false, "CanDownload": false, "SortName": strings.ToLower(l.Name), "ExternalUrls": []any{}, "Path": "",
		"EnableMediaSourceDisplay": true, "ChannelId": nil, "Taglines": []any{}, "Genres": []any{}, "PlayAccess": "Full",
		"RemoteTrailers": []any{}, "ProviderIds": map[string]any{}, "IsFolder": true, "ParentId": nil, "Type": "CollectionFolder",
		"People": []any{}, "Studios": []any{}, "GenreItems": []any{}, "LocalTrailerCount": 0, "ChildCount": l.ItemCount,
		"SpecialFeatureCount": 0, "DisplayPreferencesId": l.ID, "Tags": []any{}, "PrimaryImageAspectRatio": 16.0 / 9.0,
		"CollectionType": collectionTypes[l.Type], "ImageTags": map[string]string{}, "BackdropImageTags": []any{},
		"LocationType": "FileSystem", "MediaType": "Unknown", "LockedFields": []any{}, "LockData": false,
		"UserData": map[string]any{"PlaybackPositionTicks": 0, "PlayCount": 0, "IsFavorite": false, "Played": false, "Key": l.ID, "ItemId": l.ID},
	}
	// Use a recent backdrop as the library image.
	var id, path string
	if err := s.DB.QueryRow(`SELECT id, img_backdrop FROM items WHERE library_id=? AND img_backdrop!='' ORDER BY date_added DESC LIMIT 1`, l.ID).Scan(&id, &path); err == nil {
		d["ImageTags"] = map[string]string{"Primary": imgTag("lib" + path)}
	} else if err := s.DB.QueryRow(`SELECT id, img_primary FROM items WHERE library_id=? AND img_primary!='' ORDER BY date_added DESC LIMIT 1`, l.ID).Scan(&id, &path); err == nil {
		d["ImageTags"] = map[string]string{"Primary": imgTag("lib" + path)}
	}
	return d
}

func (s *Server) jfViews(w http.ResponseWriter, r *http.Request) error {
	libs, err := s.DB.ListLibraries()
	if err != nil {
		return err
	}
	out := []any{}
	for _, l := range libs {
		out = append(out, s.jfLibraryDto(l))
	}
	return jfJSON(w, map[string]any{"Items": out, "TotalRecordCount": len(out), "StartIndex": 0})
}

// ---- Items query ----

var jfSort = map[string]string{
	"sortname":             "i.sort_name COLLATE NOCASE",
	"name":                 "i.sort_name COLLATE NOCASE",
	"datecreated":          "i.date_added",
	"premieredate":         "i.premiere_date",
	"productionyear":       "i.year",
	"communityrating":      "i.community_rating",
	"criticrating":         "i.community_rating",
	"random":               "RANDOM()",
	"dateplayed":           "COALESCE(ud.last_played,0)",
	"playcount":            "COALESCE(ud.play_count,0)",
	"runtime":              "i.duration",
	"indexnumber":          "COALESCE(i.episode_number, i.track_number, 0)",
	"parentindexnumber":    "COALESCE(i.season_number, i.disc_number, 0)",
	"albumartist":          "i.album_artist COLLATE NOCASE",
	"album":                "i.album_id",
	"seriessortname":       "i.series_id",
	"datelastcontentadded": "i.date_added",
}

var topLevelTypes = map[string][]string{
	"movies": {db.TypeMovie}, "tvshows": {db.TypeSeries}, "music": {db.TypeMusicAlbum},
	"books": {db.TypeBook, db.TypeAudioBook}, "photos": {db.TypePhotoAlbum, db.TypePhoto},
}

func splitList(v string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == '|' }) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func inClause(col string, vals []string, args *[]any) string {
	for _, v := range vals {
		*args = append(*args, v)
	}
	return col + " IN (" + strings.TrimSuffix(strings.Repeat("?,", len(vals)), ",") + ")"
}

func (s *Server) jfQueryItems(w http.ResponseWriter, r *http.Request) error {
	u := userFrom(r)
	where := []string{"i.extra_type=''"}
	args := []any{u.ID}
	types := splitList(jfQuery(r, "IncludeItemTypes"))
	for i, t := range types {
		if strings.EqualFold(t, "Folder") {
			types[i] = db.TypeAudioBook
		}
	}
	recursive := strings.EqualFold(jfQuery(r, "Recursive"), "true")
	parent := normID(jfQuery(r, "ParentId"))
	if ids := splitList(jfQuery(r, "Ids")); len(ids) > 0 {
		for i := range ids {
			ids[i] = normID(ids[i])
		}
		where = append(where, inClause("i.id", ids, &args))
	} else if parent != "" {
		if lib, err := s.DB.GetLibrary(parent); err == nil {
			where = append(where, "i.library_id=?")
			args = append(args, lib.ID)
			if len(types) == 0 {
				if recursive {
					types = []string{db.TypeMovie, db.TypeSeries, db.TypeMusicAlbum, db.TypeBook, db.TypeAudioBook, db.TypePhoto}
				} else {
					types = topLevelTypes[lib.Type]
					if lib.Type == "photos" {
						where = append(where, "i.parent_id IS NULL")
					}
				}
			}
		} else if p, err := s.DB.GetItem(parent); err == nil {
			switch p.Type {
			case db.TypeSeries:
				if recursive || containsFold(types, db.TypeEpisode) {
					where = append(where, "i.series_id=?")
				} else {
					where = append(where, "i.parent_id=?")
				}
			case db.TypeMusicAlbum:
				where = append(where, "i.album_id=?")
			case db.TypeMusicArtist:
				if containsFold(types, db.TypeAudio) {
					where = append(where, "i.artist_id=? AND i.type='Audio'")
				} else {
					where = append(where, "i.artist_id=? AND i.type='MusicAlbum'")
				}
			default:
				where = append(where, "(i.parent_id=? OR i.season_id=?)")
				args = append(args, p.ID)
			}
			args = append(args, p.ID)
		} else {
			return s.jfQueryResult(w, r, nil, 0, 0)
		}
	} else if len(types) == 0 {
		types = []string{db.TypeMovie, db.TypeSeries}
	}
	if len(types) > 0 {
		where = append(where, inClause("i.type", types, &args))
	}
	if ex := splitList(jfQuery(r, "ExcludeItemTypes")); len(ex) > 0 {
		where = append(where, "NOT "+inClause("i.type", ex, &args))
	}
	for _, key := range []string{"AlbumArtistIds", "ArtistIds", "ContributingArtistIds"} {
		if v := splitList(jfQuery(r, key)); len(v) > 0 {
			for i := range v {
				v[i] = normID(v[i])
			}
			where = append(where, inClause("i.artist_id", v, &args))
		}
	}
	if v := splitList(jfQuery(r, "AlbumIds")); len(v) > 0 {
		for i := range v {
			v[i] = normID(v[i])
		}
		where = append(where, inClause("i.album_id", v, &args))
	}
	if v := jfQuery(r, "SeriesId"); v != "" {
		where = append(where, "i.series_id=?")
		args = append(args, normID(v))
	}
	if g := splitList(jfQuery(r, "Genres")); len(g) > 0 {
		where = append(where, "EXISTS (SELECT 1 FROM json_each(i.genres) WHERE "+inClause("value", g, &args)+")")
	}
	if v := splitList(jfQuery(r, "Years")); len(v) > 0 {
		where = append(where, inClause("i.year", v, &args))
	}
	if v := jfQuery(r, "SearchTerm"); v != "" {
		where = append(where, "(i.name LIKE ? OR i.original_title LIKE ?)")
		args = append(args, "%"+v+"%", "%"+v+"%")
	}
	if v := jfQuery(r, "NameStartsWith"); v != "" {
		where = append(where, "i.sort_name LIKE ?")
		args = append(args, v+"%")
	}
	if v := jfQuery(r, "NameLessThan"); v != "" {
		where = append(where, "i.sort_name < ?")
		args = append(args, v)
	}
	filters := strings.ToLower(jfQuery(r, "Filters"))
	if strings.Contains(filters, "isfavorite") || strings.EqualFold(jfQuery(r, "IsFavorite"), "true") {
		where = append(where, "ud.favorite=1")
	}
	if strings.Contains(filters, "isresumable") {
		where = append(where, "ud.position>0 AND ud.played=0")
	}
	if strings.Contains(filters, "isunplayed") || strings.EqualFold(jfQuery(r, "IsPlayed"), "false") {
		where = append(where, "COALESCE(ud.played,0)=0")
	} else if strings.Contains(filters, "isplayed") || strings.EqualFold(jfQuery(r, "IsPlayed"), "true") {
		where = append(where, "ud.played=1")
	}
	if strings.Contains(filters, "isnotfolder") {
		where = append(where, "i.is_folder=0")
	}
	order := jfOrder(r)
	start, _ := strconv.Atoi(jfQuery(r, "StartIndex"))
	limit, err := strconv.Atoi(jfQuery(r, "Limit"))
	if err != nil || limit <= 0 || limit > 2000 {
		limit = 2000
	}
	from := ` FROM items i LEFT JOIN user_data ud ON ud.item_id=i.id AND ud.user_id=? WHERE ` + strings.Join(where, " AND ")
	var total int
	if err := s.DB.QueryRow(`SELECT COUNT(*)`+from, args...).Scan(&total); err != nil {
		return err
	}
	items, err := s.queryItemsSQL(`SELECT `+db.ItemCols("i")+from+` ORDER BY `+order+fmt.Sprintf(" LIMIT %d OFFSET %d", limit, start), args...)
	if err != nil {
		return err
	}
	return s.jfQueryResult(w, r, items, total, start)
}

func containsFold(list []string, v string) bool {
	for _, x := range list {
		if strings.EqualFold(x, v) {
			return true
		}
	}
	return false
}

func jfOrder(r *http.Request) string {
	sorts := splitList(jfQuery(r, "SortBy"))
	orders := splitList(jfQuery(r, "SortOrder"))
	var parts []string
	for i, sname := range sorts {
		col, ok := jfSort[strings.ToLower(sname)]
		if !ok {
			continue
		}
		dir := "ASC"
		o := ""
		if i < len(orders) {
			o = orders[i]
		} else if len(orders) > 0 {
			o = orders[0]
		}
		if strings.EqualFold(o, "Descending") {
			dir = "DESC"
		}
		if col == "RANDOM()" {
			parts = append(parts, col)
		} else {
			parts = append(parts, col+" "+dir)
		}
	}
	parts = append(parts, "COALESCE(i.season_number, i.disc_number, 0)", "COALESCE(i.episode_number, i.track_number, 0)", "i.sort_name COLLATE NOCASE")
	return strings.Join(parts, ", ")
}

func (s *Server) queryItemsSQL(query string, args ...any) ([]*db.Item, error) {
	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*db.Item
	for rows.Next() {
		it, err := db.ScanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *Server) jfGetItem(w http.ResponseWriter, r *http.Request) error {
	id := normID(jfParam(r, "itemId"))
	if l, err := s.DB.GetLibrary(id); err == nil {
		return jfJSON(w, s.jfLibraryDto(l))
	}
	it, err := s.DB.GetItem(id)
	if err != nil {
		return err
	}
	c, err := s.jfContext(r, []*db.Item{it}, true)
	if err != nil {
		return err
	}
	d := s.jfItem(c, it)
	if it.Type == db.TypeSeries {
		var n int
		_ = s.DB.QueryRow(`SELECT COUNT(*) FROM items WHERE parent_id=? AND type='Season'`, it.ID).Scan(&n)
		d["ChildCount"] = n
	}
	return jfJSON(w, d)
}

func (s *Server) jfLatest(w http.ResponseWriter, r *http.Request) error {
	parent := normID(jfQuery(r, "ParentId"))
	limit, err := strconv.Atoi(jfQuery(r, "Limit"))
	if err != nil || limit <= 0 {
		limit = 20
	}
	where := "WHERE extra_type='' AND type IN ('Movie','Series','MusicAlbum','Book','AudioBook')"
	var args []any
	if parent != "" {
		where += " AND library_id=?"
		args = append(args, parent)
	}
	if types := splitList(jfQuery(r, "IncludeItemTypes")); len(types) > 0 {
		// Episodes are grouped under their series like Jellyfin does.
		for i, t := range types {
			if strings.EqualFold(t, "Episode") {
				types[i] = db.TypeSeries
			}
		}
		where += " AND " + inClause("type", types, &args)
	}
	items, err := s.DB.QueryItems(where+fmt.Sprintf(" ORDER BY date_added DESC LIMIT %d", limit), args...)
	if err != nil {
		return err
	}
	dtos, err := s.jfItems(r, items)
	if err != nil {
		return err
	}
	return jfJSON(w, dtos)
}

func (s *Server) jfResume(w http.ResponseWriter, r *http.Request) error {
	u := userFrom(r)
	types := []string{db.TypeMovie, db.TypeEpisode}
	if strings.Contains(strings.ToLower(jfQuery(r, "MediaTypes")), "audio") {
		types = []string{db.TypeAudio}
	}
	args := []any{u.ID}
	where := `JOIN user_data ud ON ud.item_id=items.id AND ud.user_id=? WHERE ud.position>0 AND ud.played=0 AND ` + inClause("items.type", types, &args)
	if p := normID(jfQuery(r, "ParentId")); p != "" {
		where += " AND items.library_id=?"
		args = append(args, p)
	}
	limit, err := strconv.Atoi(jfQuery(r, "Limit"))
	if err != nil || limit <= 0 {
		limit = 20
	}
	items, err := s.DB.QueryItems(where+fmt.Sprintf(" ORDER BY ud.last_played DESC LIMIT %d", limit), args...)
	if err != nil {
		return err
	}
	return s.jfQueryResult(w, r, items, len(items), 0)
}

func (s *Server) jfNextUp(w http.ResponseWriter, r *http.Request) error {
	u := userFrom(r)
	limit, err := strconv.Atoi(jfQuery(r, "Limit"))
	if err != nil || limit <= 0 {
		limit = 24
	}
	var items []*db.Item
	if sid := normID(jfQuery(r, "SeriesId")); sid != "" {
		eps, err := s.orderedEpisodes(sid)
		if err != nil {
			return err
		}
		ids := make([]string, len(eps))
		for i, e := range eps {
			ids[i] = e.ID
		}
		ud, _ := s.DB.GetUserData(u.ID, ids)
		last := -1
		for i, e := range eps {
			if ud[e.ID].Played {
				last = i
			}
		}
		for i := last + 1; i < len(eps); i++ {
			if !ud[eps[i].ID].Played {
				items = append(items, eps[i])
				break
			}
		}
	} else if items, err = s.nextUp(u.ID, limit); err != nil {
		return err
	}
	return s.jfQueryResult(w, r, items, len(items), 0)
}

func (s *Server) jfSeasons(w http.ResponseWriter, r *http.Request) error {
	items, err := s.DB.QueryItems(`WHERE parent_id=? AND type='Season' ORDER BY CASE WHEN season_number=0 THEN 9999 ELSE season_number END`, normID(jfParam(r, "seriesId")))
	if err != nil {
		return err
	}
	return s.jfQueryResult(w, r, items, len(items), 0)
}

func (s *Server) jfEpisodes(w http.ResponseWriter, r *http.Request) error {
	sid := normID(jfParam(r, "seriesId"))
	where := `WHERE series_id=? AND type='Episode'`
	args := []any{sid}
	if v := normID(jfQuery(r, "SeasonId")); v != "" {
		where += " AND season_id=?"
		args = append(args, v)
	} else if v := jfQuery(r, "Season"); v != "" {
		where += " AND season_number=?"
		args = append(args, v)
	}
	items, err := s.DB.QueryItems(where+` ORDER BY CASE WHEN COALESCE(season_number,1)=0 THEN 9999 ELSE COALESCE(season_number,1) END, COALESCE(episode_number,9999), sort_name`, args...)
	if err != nil {
		return err
	}
	if v := normID(jfQuery(r, "StartItemId")); v != "" {
		for i, it := range items {
			if it.ID == v {
				items = items[i:]
				break
			}
		}
	}
	return s.jfQueryResult(w, r, items, len(items), 0)
}

func (s *Server) jfSimilar(w http.ResponseWriter, r *http.Request) error {
	it, err := s.DB.GetItem(normID(jfParam(r, "itemId")))
	if err != nil {
		return err
	}
	items, err := s.similarItems(it)
	if err != nil {
		return err
	}
	return s.jfQueryResult(w, r, items, len(items), 0)
}

func (s *Server) jfSearchHints(w http.ResponseWriter, r *http.Request) error {
	term := jfQuery(r, "SearchTerm")
	like := "%" + term + "%"
	items, err := s.DB.QueryItems(`WHERE extra_type='' AND type IN ('Movie','Series','Episode','MusicAlbum','MusicArtist','Audio','Book') AND name LIKE ? ORDER BY CASE WHEN name LIKE ? THEN 0 ELSE 1 END, sort_name LIMIT 50`, like, term+"%")
	if err != nil {
		return err
	}
	c, err := s.jfContext(r, items, false)
	if err != nil {
		return err
	}
	hints := []any{}
	for _, it := range items {
		d := s.jfItem(c, it)
		h := map[string]any{"ItemId": it.ID, "Id": it.ID, "Name": it.Name, "Type": d["Type"], "MediaType": d["MediaType"],
			"IsFolder": d["IsFolder"], "RunTimeTicks": d["RunTimeTicks"], "ProductionYear": d["ProductionYear"]}
		if tags, ok := d["ImageTags"].(map[string]string); ok && tags["Primary"] != "" {
			h["PrimaryImageTag"] = tags["Primary"]
		}
		for _, k := range []string{"SeriesName", "Album", "AlbumArtist", "IndexNumber", "ParentIndexNumber"} {
			if v, ok := d[k]; ok {
				h[k] = v
			}
		}
		hints = append(hints, h)
	}
	return jfJSON(w, map[string]any{"SearchHints": hints, "TotalRecordCount": len(hints)})
}

func (s *Server) jfGenres(w http.ResponseWriter, r *http.Request) error {
	where := "WHERE i.extra_type=''"
	var args []any
	if p := normID(jfQuery(r, "ParentId")); p != "" {
		where += " AND i.library_id=?"
		args = append(args, p)
	}
	rows, err := s.DB.Query(`SELECT DISTINCT g.value FROM items i, json_each(i.genres) g `+where+` ORDER BY g.value`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var g string
		_ = rows.Scan(&g)
		out = append(out, map[string]any{"Name": g, "Id": db.StableID("genre", g), "Type": "Genre", "ServerId": s.serverID(), "ImageTags": map[string]string{}, "BackdropImageTags": []string{}})
	}
	return jfJSON(w, map[string]any{"Items": out, "TotalRecordCount": len(out), "StartIndex": 0})
}

// jfImage serves item and library images (Primary, Backdrop, Logo, Thumb).
func (s *Server) jfImage(w http.ResponseWriter, r *http.Request) error {
	id := normID(jfParam(r, "itemId"))
	kind := strings.ToLower(jfParam(r, "type"))
	var src string
	if l, err := s.DB.GetLibrary(id); err == nil {
		if err := s.DB.QueryRow(`SELECT img_backdrop FROM items WHERE library_id=? AND img_backdrop!='' ORDER BY date_added DESC LIMIT 1`, l.ID).Scan(&src); err != nil {
			_ = s.DB.QueryRow(`SELECT img_primary FROM items WHERE library_id=? AND img_primary!='' ORDER BY date_added DESC LIMIT 1`, l.ID).Scan(&src)
		}
	} else {
		it, err := s.DB.GetItem(id)
		if err != nil {
			return err
		}
		switch kind {
		case "primary":
			src = it.ImgPrimary
			if (it.Type == db.TypeEpisode || it.Type == db.TypeMusicArtist) && it.ImgThumb != "" && (src == "" || it.Type == db.TypeEpisode) {
				src = it.ImgThumb
			}
		case "backdrop":
			src = it.ImgBackdrop
		case "logo":
			src = it.ImgLogo
		case "thumb":
			src = it.ImgThumb
			if src == "" {
				src = it.ImgBackdrop
			}
		}
	}
	if src == "" {
		return errf(http.StatusNotFound, "no image")
	}
	if _, err := os.Stat(src); err != nil {
		return errf(http.StatusNotFound, "image missing")
	}
	width := 0
	for _, k := range []string{"maxWidth", "fillWidth", "width"} {
		if v, err := strconv.Atoi(jfQuery(r, k)); err == nil && v > 0 {
			width = v
			break
		}
	}
	if width == 0 {
		// Derive a width from height requests for posters.
		for _, k := range []string{"maxHeight", "fillHeight", "height"} {
			if v, err := strconv.Atoi(jfQuery(r, k)); err == nil && v > 0 {
				width = v * 2 / 3
				if kind == "backdrop" || kind == "thumb" {
					width = v * 16 / 9
				}
				break
			}
		}
	}
	out, err := s.Images.Resized(src, min(width, 3840))
	if err != nil {
		return err
	}
	w.Header().Set("Cache-Control", "public, max-age=2592000")
	http.ServeFile(w, r, out)
	return nil
}
