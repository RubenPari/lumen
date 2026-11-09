package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rubenpari/lumen/internal/db"
)

// TMDB is a minimal client for The Movie Database API v3.
type TMDB struct {
	APIKey   string
	Language string // e.g. "it-IT"
	CacheDir string
	client   *http.Client
	mu       sync.Mutex
	last     time.Time
}

// NewTMDB creates a client.
func NewTMDB(apiKey, language, cacheDir string) *TMDB {
	if language == "" {
		language = "en-US"
	}
	return &TMDB{APIKey: apiKey, Language: language, CacheDir: cacheDir, client: &http.Client{Timeout: 20 * time.Second}}
}

const tmdbBase = "https://api.themoviedb.org/3"
const tmdbImg = "https://image.tmdb.org/t/p/"

func (t *TMDB) get(ctx context.Context, path string, q url.Values, out any) error {
	// Light rate limiting (TMDB allows ~50 req/s).
	t.mu.Lock()
	if d := time.Since(t.last); d < 25*time.Millisecond {
		time.Sleep(25*time.Millisecond - d)
	}
	t.last = time.Now()
	t.mu.Unlock()
	if q == nil {
		q = url.Values{}
	}
	q.Set("language", t.Language)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, tmdbBase+path+"?"+q.Encode(), nil)
	// Support both v3 API keys and v4 read access tokens.
	if strings.HasPrefix(t.APIKey, "eyJ") {
		req.Header.Set("Authorization", "Bearer "+t.APIKey)
	} else {
		q.Set("api_key", t.APIKey)
		req.URL.RawQuery = q.Encode()
	}
	req.Header.Set("Accept", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return db.ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("tmdb %s: %s %s", path, resp.Status, b)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (t *TMDB) langs() string {
	l := strings.SplitN(t.Language, "-", 2)[0]
	if l == "en" {
		return "en,null"
	}
	return l + ",en,null"
}

type tmdbImage struct {
	FilePath string  `json:"file_path"`
	Lang     *string `json:"iso_639_1"`
	Vote     float64 `json:"vote_average"`
}

type tmdbImages struct {
	Posters   []tmdbImage `json:"posters"`
	Backdrops []tmdbImage `json:"backdrops"`
	Logos     []tmdbImage `json:"logos"`
}

type tmdbCredits struct {
	Cast []struct {
		Name        string `json:"name"`
		Character   string `json:"character"`
		ProfilePath string `json:"profile_path"`
	} `json:"cast"`
	Crew []struct {
		Name        string `json:"name"`
		Job         string `json:"job"`
		ProfilePath string `json:"profile_path"`
	} `json:"crew"`
}

type tmdbGenre struct {
	Name string `json:"name"`
}

type tmdbCompany struct {
	Name string `json:"name"`
}

// pickImage chooses the best image preferring the configured language, then
// English, then language-less images.
func (t *TMDB) pickImage(imgs []tmdbImage, preferNoLang bool) string {
	lang := strings.SplitN(t.Language, "-", 2)[0]
	score := func(i tmdbImage) float64 {
		s := i.Vote
		switch {
		case i.Lang == nil:
			if preferNoLang {
				s += 30
			} else {
				s += 5
			}
		case *i.Lang == lang:
			s += 20
		case *i.Lang == "en":
			s += 10
		}
		return s
	}
	best, bestScore := "", -1.0
	for _, i := range imgs {
		if sc := score(i); sc > bestScore {
			best, bestScore = i.FilePath, sc
		}
	}
	return best
}

func peopleFromCredits(c tmdbCredits) []db.Person {
	var out []db.Person
	for i, p := range c.Cast {
		if i >= 30 {
			break
		}
		img := ""
		if p.ProfilePath != "" {
			img = tmdbImg + "w185" + p.ProfilePath
		}
		out = append(out, db.Person{Name: p.Name, Role: p.Character, Type: "Actor", Image: img})
	}
	for _, p := range c.Crew {
		typ := ""
		switch p.Job {
		case "Director":
			typ = "Director"
		case "Screenplay", "Writer":
			typ = "Writer"
		default:
			continue
		}
		img := ""
		if p.ProfilePath != "" {
			img = tmdbImg + "w185" + p.ProfilePath
		}
		out = append(out, db.Person{Name: p.Name, Role: p.Job, Type: typ, Image: img})
	}
	return out
}

// Download fetches a TMDB image into the cache dir and returns the local path.
func (t *TMDB) Download(ctx context.Context, filePath, size, name string) (string, error) {
	if filePath == "" {
		return "", errors.New("empty path")
	}
	ext := filepath.Ext(filePath)
	dst := filepath.Join(t.CacheDir, "tmdb", name+ext)
	if _, err := os.Stat(dst); err == nil {
		return dst, nil
	}
	_ = os.MkdirAll(filepath.Dir(dst), 0o755)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, tmdbImg+size+filePath, nil)
	resp, err := t.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("image: %s", resp.Status)
	}
	tmp := dst + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return "", err
	}
	f.Close()
	return dst, os.Rename(tmp, dst)
}

func (t *TMDB) applyImages(ctx context.Context, it *db.Item, poster, backdrop, logo string) {
	if poster != "" && it.ImgPrimary == "" {
		if p, err := t.Download(ctx, poster, "w780", it.ID+"-primary"); err == nil {
			it.ImgPrimary = p
		}
	}
	if backdrop != "" && it.ImgBackdrop == "" {
		if p, err := t.Download(ctx, backdrop, "w1280", it.ID+"-backdrop"); err == nil {
			it.ImgBackdrop = p
		}
	}
	if logo != "" && it.ImgLogo == "" {
		if p, err := t.Download(ctx, logo, "w500", it.ID+"-logo"); err == nil {
			it.ImgLogo = p
		}
	}
}

func fill(dst *string, v string) {
	if *dst == "" {
		*dst = strings.TrimSpace(v)
	}
}

// FindMovie resolves the TMDB id of a movie.
func (t *TMDB) FindMovie(ctx context.Context, it *db.Item) (string, error) {
	if it.TmdbID != "" {
		return it.TmdbID, nil
	}
	if it.ImdbID != "" {
		var r struct {
			Movies []struct {
				ID int `json:"id"`
			} `json:"movie_results"`
		}
		if err := t.get(ctx, "/find/"+it.ImdbID, url.Values{"external_source": {"imdb_id"}}, &r); err == nil && len(r.Movies) > 0 {
			return strconv.Itoa(r.Movies[0].ID), nil
		}
	}
	search := func(year int) (string, error) {
		q := url.Values{"query": {it.Name}}
		if year > 0 {
			q.Set("year", strconv.Itoa(year))
		}
		var r struct {
			Results []struct {
				ID int `json:"id"`
			} `json:"results"`
		}
		if err := t.get(ctx, "/search/movie", q, &r); err != nil {
			return "", err
		}
		if len(r.Results) == 0 {
			return "", db.ErrNotFound
		}
		return strconv.Itoa(r.Results[0].ID), nil
	}
	id, err := search(it.Year)
	if errors.Is(err, db.ErrNotFound) && it.Year > 0 {
		id, err = search(0)
	}
	return id, err
}

// FetchMovie enriches a movie item from TMDB.
func (t *TMDB) FetchMovie(ctx context.Context, it *db.Item) error {
	id, err := t.FindMovie(ctx, it)
	if err != nil {
		return err
	}
	var m struct {
		ID            int           `json:"id"`
		Title         string        `json:"title"`
		OriginalTitle string        `json:"original_title"`
		Overview      string        `json:"overview"`
		Tagline       string        `json:"tagline"`
		ReleaseDate   string        `json:"release_date"`
		Runtime       int           `json:"runtime"`
		VoteAverage   float64       `json:"vote_average"`
		ImdbID        string        `json:"imdb_id"`
		PosterPath    string        `json:"poster_path"`
		BackdropPath  string        `json:"backdrop_path"`
		Genres        []tmdbGenre   `json:"genres"`
		Companies     []tmdbCompany `json:"production_companies"`
		Credits       tmdbCredits   `json:"credits"`
		Images        tmdbImages    `json:"images"`
		ReleaseDates  struct {
			Results []struct {
				Country string `json:"iso_3166_1"`
				Dates   []struct {
					Cert string `json:"certification"`
				} `json:"release_dates"`
			} `json:"results"`
		} `json:"release_dates"`
	}
	if err := t.get(ctx, "/movie/"+id, url.Values{"append_to_response": {"credits,images,release_dates"}, "include_image_language": {t.langs()}}, &m); err != nil {
		return err
	}
	it.TmdbID = strconv.Itoa(m.ID)
	if !it.MetadataLocked {
		if m.Title != "" {
			it.Name = m.Title
		}
		fill(&it.OriginalTitle, m.OriginalTitle)
		if m.Overview != "" {
			it.Overview = m.Overview
		}
		fill(&it.Tagline, m.Tagline)
		fill(&it.PremiereDate, m.ReleaseDate)
		if len(m.ReleaseDate) >= 4 {
			it.Year, _ = strconv.Atoi(m.ReleaseDate[:4])
		}
		if m.VoteAverage > 0 {
			it.CommunityRating = m.VoteAverage
		}
		if len(m.Genres) > 0 {
			it.Genres = nil
			for _, g := range m.Genres {
				it.Genres = append(it.Genres, g.Name)
			}
		}
	}
	fill(&it.ImdbID, m.ImdbID)
	if it.Duration == 0 && m.Runtime > 0 {
		it.Duration = float64(m.Runtime * 60)
	}
	if len(it.Studios) == 0 {
		for _, c := range m.Companies {
			it.Studios = append(it.Studios, c.Name)
		}
	}
	if len(it.People) == 0 {
		it.People = peopleFromCredits(m.Credits)
	}
	if it.OfficialRating == "" {
		country := "US"
		if p := strings.SplitN(t.Language, "-", 2); len(p) == 2 {
			country = p[1]
		}
		for _, pass := range []string{country, "US"} {
			for _, r := range m.ReleaseDates.Results {
				if r.Country != pass {
					continue
				}
				for _, d := range r.Dates {
					if d.Cert != "" && it.OfficialRating == "" {
						it.OfficialRating = d.Cert
					}
				}
			}
		}
	}
	poster := t.pickImage(m.Images.Posters, false)
	if poster == "" {
		poster = m.PosterPath
	}
	backdrop := t.pickImage(m.Images.Backdrops, true)
	if backdrop == "" {
		backdrop = m.BackdropPath
	}
	t.applyImages(ctx, it, poster, backdrop, t.pickImage(m.Images.Logos, false))
	return nil
}

// FetchSeries enriches a series item from TMDB.
func (t *TMDB) FetchSeries(ctx context.Context, it *db.Item) error {
	id := it.TmdbID
	if id == "" && it.ImdbID != "" {
		var r struct {
			TV []struct {
				ID int `json:"id"`
			} `json:"tv_results"`
		}
		if err := t.get(ctx, "/find/"+it.ImdbID, url.Values{"external_source": {"imdb_id"}}, &r); err == nil && len(r.TV) > 0 {
			id = strconv.Itoa(r.TV[0].ID)
		}
	}
	if id == "" {
		search := func(year int) (string, error) {
			q := url.Values{"query": {it.Name}}
			if year > 0 {
				q.Set("first_air_date_year", strconv.Itoa(year))
			}
			var r struct {
				Results []struct {
					ID int `json:"id"`
				} `json:"results"`
			}
			if err := t.get(ctx, "/search/tv", q, &r); err != nil {
				return "", err
			}
			if len(r.Results) == 0 {
				return "", db.ErrNotFound
			}
			return strconv.Itoa(r.Results[0].ID), nil
		}
		var err error
		id, err = search(it.Year)
		if errors.Is(err, db.ErrNotFound) && it.Year > 0 {
			id, err = search(0)
		}
		if err != nil {
			return err
		}
	}
	var s struct {
		ID           int           `json:"id"`
		Name         string        `json:"name"`
		OriginalName string        `json:"original_name"`
		Overview     string        `json:"overview"`
		Tagline      string        `json:"tagline"`
		FirstAir     string        `json:"first_air_date"`
		LastAir      string        `json:"last_air_date"`
		Status       string        `json:"status"`
		VoteAverage  float64       `json:"vote_average"`
		PosterPath   string        `json:"poster_path"`
		BackdropPath string        `json:"backdrop_path"`
		Genres       []tmdbGenre   `json:"genres"`
		Networks     []tmdbCompany `json:"networks"`
		Credits      tmdbCredits   `json:"credits"`
		Images       tmdbImages    `json:"images"`
		ExternalIDs  struct {
			Imdb string `json:"imdb_id"`
		} `json:"external_ids"`
		Ratings struct {
			Results []struct {
				Country string `json:"iso_3166_1"`
				Rating  string `json:"rating"`
			} `json:"results"`
		} `json:"content_ratings"`
	}
	if err := t.get(ctx, "/tv/"+id, url.Values{"append_to_response": {"credits,images,external_ids,content_ratings"}, "include_image_language": {t.langs()}}, &s); err != nil {
		return err
	}
	it.TmdbID = strconv.Itoa(s.ID)
	if !it.MetadataLocked {
		if s.Name != "" {
			it.Name = s.Name
		}
		fill(&it.OriginalTitle, s.OriginalName)
		if s.Overview != "" {
			it.Overview = s.Overview
		}
		fill(&it.Tagline, s.Tagline)
		fill(&it.PremiereDate, s.FirstAir)
		if len(s.FirstAir) >= 4 {
			it.Year, _ = strconv.Atoi(s.FirstAir[:4])
		}
		if s.VoteAverage > 0 {
			it.CommunityRating = s.VoteAverage
		}
		if len(s.Genres) > 0 {
			it.Genres = nil
			for _, g := range s.Genres {
				it.Genres = append(it.Genres, g.Name)
			}
		}
	}
	it.Status = s.Status
	if s.Status == "Ended" || s.Status == "Canceled" {
		it.EndDate = s.LastAir
	}
	fill(&it.ImdbID, s.ExternalIDs.Imdb)
	if len(it.Studios) == 0 {
		for _, n := range s.Networks {
			it.Studios = append(it.Studios, n.Name)
		}
	}
	if len(it.People) == 0 {
		it.People = peopleFromCredits(s.Credits)
	}
	if it.OfficialRating == "" {
		for _, r := range s.Ratings.Results {
			if r.Country == "US" {
				it.OfficialRating = r.Rating
			}
		}
	}
	poster := t.pickImage(s.Images.Posters, false)
	if poster == "" {
		poster = s.PosterPath
	}
	backdrop := t.pickImage(s.Images.Backdrops, true)
	if backdrop == "" {
		backdrop = s.BackdropPath
	}
	t.applyImages(ctx, it, poster, backdrop, t.pickImage(s.Images.Logos, false))
	return nil
}

// SeasonDetails holds the TMDB season data for a series.
type SeasonDetails struct {
	Name       string
	Overview   string
	AirDate    string
	PosterPath string
	Episodes   map[int]EpisodeDetails
}

// EpisodeDetails holds TMDB episode data.
type EpisodeDetails struct {
	Name, Overview, AirDate, StillPath string
	Rating                             float64
	Runtime                            int
}

// FetchSeason fetches a season of a series by TMDB id.
func (t *TMDB) FetchSeason(ctx context.Context, seriesTmdbID string, season int) (*SeasonDetails, error) {
	var s struct {
		Name       string `json:"name"`
		Overview   string `json:"overview"`
		AirDate    string `json:"air_date"`
		PosterPath string `json:"poster_path"`
		Episodes   []struct {
			Number    int     `json:"episode_number"`
			Name      string  `json:"name"`
			Overview  string  `json:"overview"`
			AirDate   string  `json:"air_date"`
			StillPath string  `json:"still_path"`
			Vote      float64 `json:"vote_average"`
			Runtime   int     `json:"runtime"`
		} `json:"episodes"`
	}
	if err := t.get(ctx, fmt.Sprintf("/tv/%s/season/%d", seriesTmdbID, season), nil, &s); err != nil {
		return nil, err
	}
	d := &SeasonDetails{Name: s.Name, Overview: s.Overview, AirDate: s.AirDate, PosterPath: s.PosterPath, Episodes: map[int]EpisodeDetails{}}
	for _, e := range s.Episodes {
		d.Episodes[e.Number] = EpisodeDetails{Name: e.Name, Overview: e.Overview, AirDate: e.AirDate, StillPath: e.StillPath, Rating: e.Vote, Runtime: e.Runtime}
	}
	return d, nil
}

// ApplySeason applies season details onto a season item.
func (t *TMDB) ApplySeason(ctx context.Context, it *db.Item, d *SeasonDetails) {
	if !it.MetadataLocked {
		if d.Name != "" {
			it.Name = d.Name
		}
		if d.Overview != "" {
			it.Overview = d.Overview
		}
		fill(&it.PremiereDate, d.AirDate)
		if len(d.AirDate) >= 4 && it.Year == 0 {
			it.Year, _ = strconv.Atoi(d.AirDate[:4])
		}
	}
	t.applyImages(ctx, it, d.PosterPath, "", "")
}

// ApplyEpisode applies episode details onto an episode item.
func (t *TMDB) ApplyEpisode(ctx context.Context, it *db.Item, e EpisodeDetails) {
	if !it.MetadataLocked {
		if e.Name != "" {
			it.Name = e.Name
		}
		if e.Overview != "" {
			it.Overview = e.Overview
		}
		fill(&it.PremiereDate, e.AirDate)
		if len(e.AirDate) >= 4 {
			it.Year, _ = strconv.Atoi(e.AirDate[:4])
		}
		if e.Rating > 0 {
			it.CommunityRating = e.Rating
		}
	}
	if it.Duration == 0 && e.Runtime > 0 {
		it.Duration = float64(e.Runtime * 60)
	}
	if e.StillPath != "" && it.ImgThumb == "" {
		if p, err := t.Download(ctx, e.StillPath, "w780", it.ID+"-thumb"); err == nil {
			it.ImgThumb = p
		}
	}
}
