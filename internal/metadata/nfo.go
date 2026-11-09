// Package metadata contains local (NFO, images) and remote (TMDB) metadata providers.
package metadata

import (
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/rubenpari/lumen/internal/db"
)

// NFO is the subset of Kodi NFO fields supported (port of MediaBrowser.XbmcMetadata).
type NFO struct {
	Title         string   `xml:"title"`
	OriginalTitle string   `xml:"originaltitle"`
	SortTitle     string   `xml:"sorttitle"`
	Plot          string   `xml:"plot"`
	Outline       string   `xml:"outline"`
	Tagline       string   `xml:"tagline"`
	Year          string   `xml:"year"`
	Premiered     string   `xml:"premiered"`
	Aired         string   `xml:"aired"`
	ReleaseDate   string   `xml:"releasedate"`
	Rating        string   `xml:"rating"`
	MPAA          string   `xml:"mpaa"`
	Runtime       string   `xml:"runtime"`
	Genres        []string `xml:"genre"`
	Studios       []string `xml:"studio"`
	Directors     []string `xml:"director"`
	Writers       []string `xml:"credits"`
	Status        string   `xml:"status"`
	Season        string   `xml:"season"`
	Episode       string   `xml:"episode"`
	EpisodeEnd    string   `xml:"episodenumberend"`
	TmdbID        string   `xml:"tmdbid"`
	ImdbID        string   `xml:"imdbid"`
	ID            string   `xml:"id"`
	LockData      string   `xml:"lockdata"`
	Ratings       struct {
		Rating []struct {
			Name    string `xml:"name,attr"`
			Default bool   `xml:"default,attr"`
			Value   string `xml:"value"`
		} `xml:"rating"`
	} `xml:"ratings"`
	UniqueIDs []struct {
		Type  string `xml:"type,attr"`
		Value string `xml:",chardata"`
	} `xml:"uniqueid"`
	Actors []struct {
		Name  string `xml:"name"`
		Role  string `xml:"role"`
		Thumb string `xml:"thumb"`
	} `xml:"actor"`
}

// ReadNFO parses an NFO file. Kodi NFO files may contain a trailing URL after
// the XML document, so parsing stops at the first root element.
func ReadNFO(path string) (*NFO, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	dec.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	var root struct {
		NFO
	}
	if err := dec.Decode(&root); err != nil {
		return nil, err
	}
	n := root.NFO
	return &n, nil
}

func firstInt(vals ...string) int {
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if len(v) >= 4 {
			if n, err := strconv.Atoi(v[:4]); err == nil && n > 1800 {
				return n
			}
		}
	}
	return 0
}

// Apply copies NFO values onto an item (only non-empty values win).
func (n *NFO) Apply(it *db.Item) {
	set := func(dst *string, v string) {
		if v = strings.TrimSpace(v); v != "" {
			*dst = v
		}
	}
	set(&it.Name, n.Title)
	set(&it.OriginalTitle, n.OriginalTitle)
	set(&it.SortName, n.SortTitle)
	set(&it.Overview, n.Plot)
	if it.Overview == "" {
		set(&it.Overview, n.Outline)
	}
	set(&it.Tagline, n.Tagline)
	set(&it.OfficialRating, n.MPAA)
	set(&it.Status, n.Status)
	date := strings.TrimSpace(n.Premiered)
	if date == "" {
		date = strings.TrimSpace(n.Aired)
	}
	if date == "" {
		date = strings.TrimSpace(n.ReleaseDate)
	}
	if date != "" {
		it.PremiereDate = date
	}
	if y := firstInt(n.Year, date); y > 0 {
		it.Year = y
	}
	rating := n.Rating
	for _, r := range n.Ratings.Rating {
		if r.Default || rating == "" {
			rating = r.Value
		}
	}
	if f, err := strconv.ParseFloat(strings.TrimSpace(strings.ReplaceAll(rating, ",", ".")), 64); err == nil && f > 0 {
		it.CommunityRating = f
	}
	if m, err := strconv.Atoi(strings.TrimSpace(n.Runtime)); err == nil && m > 0 && it.Duration == 0 {
		it.Duration = float64(m * 60)
	}
	if len(n.Genres) > 0 {
		var g []string
		for _, s := range n.Genres {
			for _, p := range strings.Split(s, "/") {
				if p = strings.TrimSpace(p); p != "" {
					g = append(g, p)
				}
			}
		}
		it.Genres = g
	}
	if len(n.Studios) > 0 {
		it.Studios = n.Studios
	}
	var people []db.Person
	for _, a := range n.Actors {
		if a.Name != "" {
			people = append(people, db.Person{Name: a.Name, Role: a.Role, Type: "Actor", Image: a.Thumb})
		}
	}
	for _, d := range n.Directors {
		if d = strings.TrimSpace(d); d != "" {
			people = append(people, db.Person{Name: d, Type: "Director"})
		}
	}
	for _, w := range n.Writers {
		if w = strings.TrimSpace(w); w != "" {
			people = append(people, db.Person{Name: w, Type: "Writer"})
		}
	}
	if len(people) > 0 {
		it.People = people
	}
	for _, u := range n.UniqueIDs {
		switch strings.ToLower(u.Type) {
		case "tmdb":
			set(&it.TmdbID, u.Value)
		case "imdb":
			set(&it.ImdbID, u.Value)
		}
	}
	set(&it.TmdbID, n.TmdbID)
	set(&it.ImdbID, n.ImdbID)
	if strings.HasPrefix(n.ID, "tt") {
		set(&it.ImdbID, n.ID)
	}
	if s, err := strconv.Atoi(strings.TrimSpace(n.Season)); err == nil && it.Type == db.TypeEpisode {
		it.SeasonNumber = &s
	}
	if e, err := strconv.Atoi(strings.TrimSpace(n.Episode)); err == nil && it.Type == db.TypeEpisode {
		it.EpisodeNumber = &e
	}
	if strings.EqualFold(strings.TrimSpace(n.LockData), "true") {
		it.MetadataLocked = true
	}
}
