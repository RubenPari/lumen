package scanner

import "testing"

func ip(v int) *int { return &v }

func TestEpisodeTitle(t *testing.T) {
	cases := []struct {
		file, series string
		num          *int
		want         string
	}{
		{"/tv/Breaking Bad/Season 01/Breaking.Bad.S01E01.Pilot.720p.mkv", "Breaking Bad", ip(1), "Pilot"},
		{"/tv/Breaking Bad/Season 02/Breaking Bad - 2x01 - Seven Thirty-Seven.mkv", "Breaking Bad", ip(1), "Seven Thirty-Seven"},
		{"/tv/Dark/Dark.S01E01.mkv", "Dark", ip(1), "Episode 1"},
		{"/tv/Abyss/[HorribleSubs] Made in Abyss - 01 [720p].mkv", "Made in Abyss", ip(1), "Episode 1"},
		{"/tv/Show/S02E02-E03 - Grilled.mkv", "Show", ip(2), "Grilled"},
		{"/tv/Dark/Specials/Dark - Making of.mkv", "Dark", nil, "Making of"},
	}
	for _, c := range cases {
		if got := episodeTitle(c.file, c.series, c.num); got != c.want {
			t.Errorf("%s: got %q want %q", c.file, got, c.want)
		}
	}
}

func TestDisplayName(t *testing.T) {
	for in, want := range map[string]string{
		"Crouching.Tiger.Hidden.Dragon": "Crouching Tiger Hidden Dragon",
		"[HorribleSubs] Made in Abyss":  "Made in Abyss",
		"The Matrix":                    "The Matrix",
		"St. Vincent":                   "St. Vincent",
	} {
		if got := displayName(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}
