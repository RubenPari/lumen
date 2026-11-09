package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/rubenpari/lumen/internal/db"
)

// TestSharedItemListingAndHydration drives the native and Jellyfin item
// endpoints through Server.Handler. Shared filters must agree on ids, order,
// and total. Filters that belong to one API stay on that API. Series
// hydration keeps the fields each payload already exposes.
func TestSharedItemListingAndHydration(t *testing.T) {
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	if _, err := database.Exec(`INSERT INTO users(id,name,password_hash,is_admin,created_at) VALUES('u1','admin','',1,1)`); err != nil {
		t.Fatal(err)
	}
	now := db.Now()
	if _, err := database.Exec(`INSERT INTO sessions(token,user_id,device,created_at,last_seen) VALUES('tok','u1','test',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	for _, l := range []*db.Library{
		{ID: "libm", Name: "Movies", Type: "movies", Paths: []string{"/m"}, CreatedAt: 1},
		{ID: "libm2", Name: "More Movies", Type: "movies", Paths: []string{"/m2"}, CreatedAt: 2},
		{ID: "libt", Name: "TV", Type: "tvshows", Paths: []string{"/t"}, CreatedAt: 3},
		{ID: "libb", Name: "Books", Type: "books", Paths: []string{"/b"}, CreatedAt: 4},
	} {
		if err := database.SaveLibrary(l); err != nil {
			t.Fatal(err)
		}
	}

	mustItem := func(it *db.Item) {
		t.Helper()
		if it.Path == "" {
			it.Path = it.ID
		}
		if err := database.UpsertItem(it); err != nil {
			t.Fatal(err)
		}
	}
	sn1, sn2 := 1, 2
	ep1, ep2 := 1, 2
	// Ids have no hyphens: Jellyfin normalizes ParentId and SeriesId by stripping dashes.
	mustItem(&db.Item{ID: "malpha", LibraryID: "libm", Type: db.TypeMovie, Name: "Alpha", Year: 2010, Genres: []string{"Action"}})
	mustItem(&db.Item{ID: "mbravo", LibraryID: "libm", Type: db.TypeMovie, Name: "Bravo", OriginalTitle: "Alpha Night", Year: 2011, Genres: []string{"Drama"}})
	mustItem(&db.Item{ID: "mcharlie", LibraryID: "libm", Type: db.TypeMovie, Name: "Charlie", Year: 2010, Genres: []string{"Action"}})
	mustItem(&db.Item{ID: "mdelta", LibraryID: "libm", Type: db.TypeMovie, Name: "Delta", Year: 1995, Genres: []string{"Comedy"}})
	mustItem(&db.Item{ID: "mzulu", LibraryID: "libm2", Type: db.TypeMovie, Name: "Zulu", Year: 2001, Genres: []string{"Horror"}})
	mustItem(&db.Item{ID: "abspoken", LibraryID: "libb", Type: db.TypeAudioBook, Name: "Spoken", IsFolder: true})

	mustItem(&db.Item{ID: "sharbor", LibraryID: "libt", Type: db.TypeSeries, Name: "Harbor", Year: 2010, Genres: []string{"Action"}, IsFolder: true})
	mustItem(&db.Item{ID: "seh1", LibraryID: "libt", Type: db.TypeSeason, Name: "Season 1", ParentID: "sharbor", SeriesID: "sharbor", SeasonNumber: &sn1, IsFolder: true})
	mustItem(&db.Item{ID: "epilot", LibraryID: "libt", Type: db.TypeEpisode, Name: "Pilot", ParentID: "seh1", SeriesID: "sharbor", SeasonID: "seh1", SeasonNumber: &sn1, EpisodeNumber: &ep1})
	mustItem(&db.Item{ID: "esecond", LibraryID: "libt", Type: db.TypeEpisode, Name: "Second", ParentID: "seh1", SeriesID: "sharbor", SeasonID: "seh1", SeasonNumber: &sn1, EpisodeNumber: &ep2})
	mustItem(&db.Item{ID: "seh2", LibraryID: "libt", Type: db.TypeSeason, Name: "Season 2", ParentID: "sharbor", SeriesID: "sharbor", SeasonNumber: &sn2, IsFolder: true})
	mustItem(&db.Item{ID: "ethird", LibraryID: "libt", Type: db.TypeEpisode, Name: "Third", ParentID: "seh2", SeriesID: "sharbor", SeasonID: "seh2", SeasonNumber: &sn2, EpisodeNumber: &ep1})

	mustItem(&db.Item{ID: "sclosed", LibraryID: "libt", Type: db.TypeSeries, Name: "Closed", Year: 2008, IsFolder: true})
	mustItem(&db.Item{ID: "sec1", LibraryID: "libt", Type: db.TypeSeason, Name: "Season 1", ParentID: "sclosed", SeriesID: "sclosed", SeasonNumber: &sn1, IsFolder: true})
	mustItem(&db.Item{ID: "eonly", LibraryID: "libt", Type: db.TypeEpisode, Name: "Only", ParentID: "sec1", SeriesID: "sclosed", SeasonID: "sec1", SeasonNumber: &sn1, EpisodeNumber: &ep1})

	saveUD := func(itemID string, u db.UserData) {
		t.Helper()
		if err := database.SaveUserData("u1", itemID, u); err != nil {
			t.Fatal(err)
		}
	}
	saveUD("malpha", db.UserData{Favorite: true})
	saveUD("mbravo", db.UserData{Played: true, PlayCount: 1})
	saveUD("mcharlie", db.UserData{Position: 45, Played: false})
	saveUD("epilot", db.UserData{Played: true, PlayCount: 1})
	saveUD("esecond", db.UserData{Position: 12.5})
	saveUD("eonly", db.UserData{Played: true, PlayCount: 1})

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := &Server{DB: database, Log: log, CacheDir: dir}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	type nativeItem struct {
		ID            string `json:"id"`
		SeriesName    string `json:"seriesName"`
		SeasonName    string `json:"seasonName"`
		EpisodeCount  int    `json:"episodeCount"`
		UnplayedCount int    `json:"unplayedCount"`
		UserData      struct {
			Played   bool    `json:"played"`
			Position float64 `json:"position"`
		} `json:"userData"`
	}
	type nativePage struct {
		Items []nativeItem `json:"items"`
		Total int          `json:"total"`
	}
	type jfItem struct {
		ID         string `json:"Id"`
		Name       string `json:"Name"`
		SeriesName string `json:"SeriesName"`
		Type       string `json:"Type"`
		UserData   struct {
			Played                bool    `json:"Played"`
			PlaybackPositionTicks float64 `json:"PlaybackPositionTicks"`
		} `json:"UserData"`
	}
	type jfPage struct {
		Items            []jfItem `json:"Items"`
		TotalRecordCount int      `json:"TotalRecordCount"`
	}

	getJSON := func(url string, out any) {
		t.Helper()
		resp, err := http.Get(url)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: status %d body %s", url, resp.StatusCode, body)
		}
		if err := json.Unmarshal(body, out); err != nil {
			t.Fatalf("%s: %v body %s", url, err, body)
		}
	}
	native := func(q string) nativePage {
		t.Helper()
		var page nativePage
		getJSON(ts.URL+"/api/items?"+q+"&token=tok", &page)
		return page
	}
	jellyfin := func(q string) jfPage {
		t.Helper()
		var page jfPage
		getJSON(ts.URL+"/Items?"+q+"&api_key=tok", &page)
		return page
	}
	nativeIDs := func(page nativePage) []string {
		ids := make([]string, len(page.Items))
		for i, it := range page.Items {
			ids[i] = it.ID
		}
		return ids
	}
	jfIDs := func(page jfPage) []string {
		ids := make([]string, len(page.Items))
		for i, it := range page.Items {
			ids[i] = it.ID
		}
		return ids
	}
	movies := []string{"malpha", "mbravo", "mcharlie", "mdelta", "mzulu"}
	libMovies := []string{"malpha", "mbravo", "mcharlie", "mdelta"}
	cases := []struct {
		name   string
		native string
		jf     string
		want   []string
		total  int
	}{
		{"type", "type=Movie&sort=name", "IncludeItemTypes=Movie&SortBy=SortName&SortOrder=Ascending", movies, len(movies)},
		{"library", "libraryId=libm&type=Movie&sort=name", "ParentId=libm&IncludeItemTypes=Movie&SortBy=SortName&SortOrder=Ascending", libMovies, len(libMovies)},
		{"parent", "parentId=seh1&type=Episode&sort=name", "ParentId=seh1&IncludeItemTypes=Episode&SortBy=SortName&SortOrder=Ascending", []string{"epilot", "esecond"}, 2},
		{"genre", "genre=Action&type=Movie&sort=name", "Genres=Action&IncludeItemTypes=Movie&SortBy=SortName&SortOrder=Ascending", []string{"malpha", "mcharlie"}, 2},
		{"search", "search=Alpha&type=Movie&sort=name", "SearchTerm=Alpha&IncludeItemTypes=Movie&SortBy=SortName&SortOrder=Ascending", []string{"malpha", "mbravo"}, 2},
		{"favorite", "filter=favorites&type=Movie&sort=name", "Filters=IsFavorite&IncludeItemTypes=Movie&SortBy=SortName&SortOrder=Ascending", []string{"malpha"}, 1},
		{"played", "filter=played&type=Movie&sort=name", "Filters=IsPlayed&IncludeItemTypes=Movie&SortBy=SortName&SortOrder=Ascending", []string{"mbravo"}, 1},
		{"resumable", "filter=resumable&type=Movie&sort=name", "Filters=IsResumable&IncludeItemTypes=Movie&SortBy=SortName&SortOrder=Ascending", []string{"mcharlie"}, 1},
		{"year", "year=2010&type=Movie&sort=name", "Years=2010&IncludeItemTypes=Movie&SortBy=SortName&SortOrder=Ascending", []string{"malpha", "mcharlie"}, 2},
		{"limit-offset", "libraryId=libm&type=Movie&sort=name&limit=1&offset=1", "ParentId=libm&IncludeItemTypes=Movie&SortBy=SortName&SortOrder=Ascending&Limit=1&StartIndex=1", []string{"mbravo"}, len(libMovies)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n, j := native(tc.native), jellyfin(tc.jf)
			gotN, gotJ := nativeIDs(n), jfIDs(j)
			if n.Total != tc.total || j.TotalRecordCount != tc.total || !reflect.DeepEqual(gotN, tc.want) || !reflect.DeepEqual(gotJ, tc.want) {
				t.Fatalf("native ids %v total %d; jellyfin ids %v total %d; want %v total %d", gotN, n.Total, gotJ, j.TotalRecordCount, tc.want, tc.total)
			}
		})
	}

	// Native-only: first letter of the sort name. Jellyfin has no letter filter.
	letter := native("type=Movie&letter=D&sort=name")
	if letter.Total != 1 || !reflect.DeepEqual(nativeIDs(letter), []string{"mdelta"}) {
		t.Fatalf("letter filter: %+v", letter)
	}

	// Native unplayed on a series follows episodes, not the series row.
	// Harbor still has unplayed episodes. Closed is fully watched.
	unplayed := native("type=Series&filter=unplayed&sort=name")
	if unplayed.Total != 1 || !reflect.DeepEqual(nativeIDs(unplayed), []string{"sharbor"}) {
		t.Fatalf("series-aware unplayed: %+v", unplayed)
	}

	// Jellyfin-only: sort-name prefix, and Folder meaning audiobook.
	prefix := jellyfin("NameStartsWith=Del&IncludeItemTypes=Movie&SortBy=SortName")
	if prefix.TotalRecordCount != 1 || !reflect.DeepEqual(jfIDs(prefix), []string{"mdelta"}) {
		t.Fatalf("NameStartsWith: %+v", prefix)
	}
	folders := jellyfin("IncludeItemTypes=Folder&SortBy=SortName")
	if folders.TotalRecordCount != 1 || !reflect.DeepEqual(jfIDs(folders), []string{"abspoken"}) || folders.Items[0].Type != "Folder" {
		t.Fatalf("Folder audiobook: %+v", folders)
	}

	series := native("type=Series&sort=name")
	if series.Total != 2 {
		t.Fatalf("series list: %+v", series)
	}
	byID := map[string]nativeItem{}
	for _, it := range series.Items {
		byID[it.ID] = it
	}
	harbor, closed := byID["sharbor"], byID["sclosed"]
	if harbor.EpisodeCount != 3 || harbor.UnplayedCount != 2 || harbor.UserData.Played {
		t.Fatalf("harbor hydration: %+v", harbor)
	}
	if closed.EpisodeCount != 1 || closed.UnplayedCount != 0 || !closed.UserData.Played {
		t.Fatalf("closed hydration: %+v", closed)
	}
	eps := native("type=Episode&seriesId=sharbor&sort=name")
	if eps.Total != 3 {
		t.Fatalf("episodes: %+v", eps)
	}
	for _, it := range eps.Items {
		if it.SeriesName != "Harbor" || it.SeasonName == "" {
			t.Fatalf("episode names: %+v", it)
		}
	}
	seasonByEpisode := map[string]string{}
	for _, it := range eps.Items {
		seasonByEpisode[it.ID] = it.SeasonName
	}
	if seasonByEpisode["epilot"] != "Season 1" || seasonByEpisode["esecond"] != "Season 1" || seasonByEpisode["ethird"] != "Season 2" {
		t.Fatalf("season names: %+v", seasonByEpisode)
	}

	jfSeries := jellyfin("IncludeItemTypes=Series&SortBy=SortName&SortOrder=Ascending")
	if jfSeries.TotalRecordCount != 2 {
		t.Fatalf("jellyfin series: %+v", jfSeries)
	}
	jfByID := map[string]jfItem{}
	for _, it := range jfSeries.Items {
		jfByID[it.ID] = it
	}
	if jfByID["sharbor"].UserData.Played || !jfByID["sclosed"].UserData.Played {
		t.Fatalf("jellyfin series played: harbor %+v closed %+v", jfByID["sharbor"].UserData, jfByID["sclosed"].UserData)
	}
	jfEps := jellyfin("IncludeItemTypes=Episode&SeriesId=sharbor&SortBy=SortName&SortOrder=Ascending")
	if jfEps.TotalRecordCount != 3 {
		t.Fatalf("jellyfin episodes: %+v", jfEps)
	}
	var second jfItem
	for _, it := range jfEps.Items {
		if it.SeriesName != "Harbor" {
			t.Fatalf("jellyfin series name: %+v", it)
		}
		if it.ID == "esecond" {
			second = it
		}
	}
	if second.ID == "" || second.UserData.PlaybackPositionTicks != 125_000_000 {
		t.Fatalf("playback ticks: %+v", second)
	}
}
