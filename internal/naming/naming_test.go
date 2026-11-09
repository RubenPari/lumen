package naming

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func fmtp(p *int) string {
	if p == nil {
		return "nil"
	}
	return fmt.Sprint(*p)
}

func eqp(a, b *int) bool { return fmtp(a) == fmtp(b) }

func TestEpisodeNumber(t *testing.T) {
	for _, c := range episodeNumberCases {
		r := ParseEpisodePath(c.path, false, true)
		if !eqp(r.EpisodeNumber, c.want) {
			t.Errorf("%q: got %s want %s", c.path, fmtp(r.EpisodeNumber), fmtp(c.want))
		}
	}
}

func TestSeasonNumberFromEpisode(t *testing.T) {
	for _, c := range seasonNumberCases {
		r := ResolveEpisode(c.path, false)
		var got *int
		if r != nil {
			got = r.SeasonNumber
		}
		if !eqp(got, c.want) {
			t.Errorf("%q: got %s want %s", c.path, fmtp(got), fmtp(c.want))
		}
	}
}

func TestMultiEpisode(t *testing.T) {
	for _, c := range multiEpisodeCases {
		r := ParseEpisodePath(c.path, false, true)
		if !eqp(r.EndingEpisodeNumber, c.want) {
			t.Errorf("%q: got %s want %s", c.path, fmtp(r.EndingEpisodeNumber), fmtp(c.want))
		}
	}
}

func TestSeriesResolver(t *testing.T) {
	for _, c := range seriesResolverCases {
		if got := ResolveSeries(c.path).Name; got != c.want {
			t.Errorf("%q: got %q want %q", c.path, got, c.want)
		}
	}
}

func TestSimpleEpisode(t *testing.T) {
	for _, c := range simpleEpisodeCases {
		r := ResolveEpisode(c.path, false)
		if r == nil {
			t.Errorf("%q: nil", c.path)
			continue
		}
		if !eqp(r.SeasonNumber, c.season) || !eqp(r.EpisodeNumber, c.ep) || !strings.EqualFold(r.SeriesName, c.series) {
			t.Errorf("%q: got %q S%s E%s want %q S%s E%s", c.path, r.SeriesName, fmtp(r.SeasonNumber), fmtp(r.EpisodeNumber), c.series, fmtp(c.season), fmtp(c.ep))
		}
	}
}

func testSeasonPath(t *testing.T, cases []struct {
	path     string
	parent   *string
	want     *int
	isSeason bool
}, aliases bool) {
	for _, c := range cases {
		parent := ""
		if c.parent != nil {
			parent = *c.parent
		}
		n, ok := ParseSeasonPath(c.path, parent, aliases, aliases)
		var got *int
		if ok {
			got = &n
		}
		if !eqp(got, c.want) || ok != c.isSeason {
			t.Errorf("%q: got %s/%v want %s/%v", c.path, fmtp(got), ok, fmtp(c.want), c.isSeason)
		}
	}
}

func TestSeasonPath(t *testing.T)      { testSeasonPath(t, seasonPathCases, true) }
func TestSeasonPathMixed(t *testing.T) { testSeasonPath(t, seasonPathMixedCases, false) }

func TestCleanDateTime(t *testing.T) {
	for _, c := range cleanDateTimeCases {
		in := c.in
		if in != "" {
			in = filepath.Base(in)
		}
		name, year := CleanDateTime(in)
		if !strings.EqualFold(name, c.name) || year != c.year {
			t.Errorf("%q: got %q %d want %q %d", c.in, name, year, c.name, c.year)
		}
	}
}

func TestCleanString(t *testing.T) {
	for _, c := range cleanStringCases {
		got, ok := CleanString(c.in)
		if !ok || got != c.want {
			t.Errorf("%q: got %q want %q", c.in, got, c.want)
		}
	}
}
