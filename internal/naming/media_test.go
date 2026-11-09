package naming

import "testing"

func TestMultiDiscAlbum(t *testing.T) {
	for _, c := range multiDiscCases {
		if got := IsMultiPartAlbumFolder(c.path); got != c.want {
			t.Errorf("%q: got %v want %v", c.path, got, c.want)
		}
	}
}

func TestBooks(t *testing.T) {
	for _, c := range bookCases {
		r := ParseBookName(c.in)
		if r.Name != c.name || r.SeriesName != c.series || !eqp(r.Index, c.index) || !eqp(r.Year, c.year) {
			t.Errorf("%q: got %q %q %s %s", c.in, r.Name, r.SeriesName, fmtp(r.Index), fmtp(r.Year))
		}
	}
}

func TestComics(t *testing.T) {
	if len(comicCases) == 0 {
		t.Fatal("no comic cases")
	}
	for _, c := range comicCases {
		r := ParseBookName(c.in)
		if r.Name != c.name || r.SeriesName != c.series || !eqp(r.Index, c.chapter) || !eqp(r.ParentIndex, c.volume) || !eqp(r.Year, c.year) {
			t.Errorf("%q: got %q %q ch=%s vol=%s y=%s", c.in, r.Name, r.SeriesName, fmtp(r.Index), fmtp(r.ParentIndex), fmtp(r.Year))
		}
	}
}

func TestAudioBookFiles(t *testing.T) {
	cases := []struct {
		path          string
		chapter, part *int
	}{
		{"/server/AudioBooks/Larry Potter/Larry Potter.mp3", nil, nil},
		{"/server/AudioBooks/Berry Potter/Chapter 1 .ogg", ip(1), nil},
		{"/server/AudioBooks/Nerry Potter/Part 3 - Chapter 2.mp3", ip(2), ip(3)},
	}
	for _, c := range cases {
		r := ResolveAudioBookFile(c.path)
		if r == nil || !eqp(r.ChapterNumber, c.chapter) || !eqp(r.PartNumber, c.part) {
			t.Errorf("%q: %+v", c.path, r)
		}
	}
	if ResolveAudioBookFile("/a/b.mkv") != nil {
		t.Error("video should not resolve")
	}
	if n, y := ParseAudioBookName("Batman (2020)"); n != "Batman" || y != 2020 {
		t.Errorf("got %q %d", n, y)
	}
}
