package scanner

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/rubenpari/lumen/internal/db"
)

// id3 builds a minimal ID3v2.3 tagged "mp3" file.
func id3(frames map[string]string) []byte {
	var body bytes.Buffer
	for id, v := range frames {
		data := append([]byte{0}, []byte(v)...) // ISO-8859-1 encoding
		body.WriteString(id)
		_ = binary.Write(&body, binary.BigEndian, uint32(len(data)))
		body.Write([]byte{0, 0})
		body.Write(data)
	}
	size := body.Len()
	syncsafe := []byte{byte(size >> 21 & 0x7f), byte(size >> 14 & 0x7f), byte(size >> 7 & 0x7f), byte(size & 0x7f)}
	var out bytes.Buffer
	out.WriteString("ID3")
	out.Write([]byte{3, 0, 0})
	out.Write(syncsafe)
	out.Write(body.Bytes())
	out.Write(bytes.Repeat([]byte{0xff, 0xfb, 0x90, 0x00}, 64))
	return out.Bytes()
}

func write(t *testing.T, p string, data []byte) {
	t.Helper()
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func zipBytes(t *testing.T, files map[string][]byte) []byte {
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for n, d := range files {
		w, _ := zw.Create(n)
		_, _ = w.Write(d)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func pngBytes(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		img.Set(x, 0, color.White)
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func scanLib(t *testing.T, typ string, setup func(root string)) *db.DB {
	t.Helper()
	s, d := newTestScanner(t)
	s.CacheDir = t.TempDir()
	root := t.TempDir()
	setup(root)
	lib := &db.Library{ID: typ, Name: typ, Type: typ, Paths: []string{root}, CreatedAt: 1}
	_ = d.SaveLibrary(lib)
	if err := s.scanLibrary(context.Background(), lib); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestMusicLibrary(t *testing.T) {
	d := scanLib(t, "music", func(root string) {
		write(t, filepath.Join(root, "Daft Punk", "Discovery (2001)", "01 - One More Time.mp3"),
			id3(map[string]string{"TIT2": "One More Time", "TALB": "Discovery", "TPE1": "Daft Punk", "TRCK": "1", "TYER": "2001", "TCON": "Electronic"}))
		write(t, filepath.Join(root, "Daft Punk", "Discovery (2001)", "02 - Aerodynamic.mp3"),
			id3(map[string]string{"TIT2": "Aerodynamic", "TALB": "Discovery", "TPE1": "Daft Punk", "TRCK": "2"}))
		write(t, filepath.Join(root, "Daft Punk", "Discovery (2001)", "cover.jpg"), pngBytes(10, 10))
		write(t, filepath.Join(root, "Pink Floyd", "The Wall", "CD 2", "01 Hey You.mp3"), id3(map[string]string{"TPE1": "Pink Floyd"}))
		write(t, filepath.Join(root, "Pink Floyd", "The Wall", "CD 1", "01 In the Flesh.mp3"), id3(map[string]string{"TPE1": "Pink Floyd"}))
	})
	if n := count(t, d, db.TypeMusicArtist); n != 2 {
		t.Fatalf("artists: %d", n)
	}
	albums, _ := d.QueryItems(`WHERE type='MusicAlbum' ORDER BY name`)
	if len(albums) != 2 || albums[0].Name != "Discovery" || albums[0].Year != 2001 || albums[0].ImgPrimary == "" || albums[0].AlbumArtist != "Daft Punk" {
		t.Fatalf("albums: %+v", albums)
	}
	if albums[1].Name != "The Wall" {
		t.Fatalf("multi-disc album: %s", albums[1].Name)
	}
	tracks, _ := d.QueryItems(`WHERE album_id=? ORDER BY disc_number, track_number`, albums[1].ID)
	if len(tracks) != 2 || tracks[0].Name != "In the Flesh" || *tracks[0].DiscNumber != 1 || *tracks[1].DiscNumber != 2 {
		t.Fatalf("tracks: %d %+v", len(tracks), tracks)
	}
	first, _ := d.QueryItems(`WHERE album_id=? ORDER BY track_number`, albums[0].ID)
	if first[0].Name != "One More Time" || first[0].Genres[0] != "Electronic" {
		t.Fatalf("tags not read: %+v", first[0])
	}
}

func TestPhotoLibrary(t *testing.T) {
	d := scanLib(t, "photos", func(root string) {
		var b bytes.Buffer
		_ = jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 40, 30)), nil)
		write(t, filepath.Join(root, "Vacanze 2024", "IMG_001.jpg"), b.Bytes())
		write(t, filepath.Join(root, "Vacanze 2024", "IMG_002.png"), pngBytes(20, 50))
		write(t, filepath.Join(root, "loose.jpg"), b.Bytes())
	})
	if n := count(t, d, db.TypePhoto); n != 3 {
		t.Fatalf("photos: %d", n)
	}
	albums, _ := d.QueryItems(`WHERE type='PhotoAlbum'`)
	if len(albums) != 1 || albums[0].Name != "Vacanze 2024" || albums[0].ImgPrimary == "" {
		t.Fatalf("albums: %+v", albums)
	}
	p, _ := d.QueryItems(`WHERE type='Photo' AND name='IMG_002'`)
	if p[0].Width != 20 || p[0].Height != 50 || p[0].AlbumID != albums[0].ID {
		t.Fatalf("photo: %+v", p[0])
	}
}

func TestBooksLibrary(t *testing.T) {
	opf := `<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" xmlns:dc="http://purl.org/dc/elements/1.1/" version="3.0">
<metadata><dc:title>Il nome della rosa</dc:title><dc:creator>Umberto Eco</dc:creator><dc:description>&lt;p&gt;Un giallo&lt;/p&gt;</dc:description><dc:date>1980-01-01</dc:date><dc:language>it</dc:language></metadata>
<manifest><item id="c" href="images/cover.png" media-type="image/png" properties="cover-image"/></manifest></package>`
	d := scanLib(t, "books", func(root string) {
		write(t, filepath.Join(root, "Eco", "rosa.epub"), zipBytes(t, map[string][]byte{
			"META-INF/container.xml": []byte(`<container><rootfiles><rootfile full-path="OEBPS/content.opf"/></rootfiles></container>`),
			"OEBPS/content.opf":      []byte(opf),
			"OEBPS/images/cover.png": pngBytes(30, 45),
		}))
		write(t, filepath.Join(root, "Comics", "Captain Marvel Adventures v01 c120.cbz"), zipBytes(t, map[string][]byte{
			"page10.png": pngBytes(5, 5), "page2.png": pngBytes(5, 5), "page1.png": pngBytes(5, 5),
		}))
		write(t, filepath.Join(root, "Audiobooks", "Dune (1965)", "Chapter 2.mp3"), id3(map[string]string{"TPE1": "Frank Herbert"}))
		write(t, filepath.Join(root, "Audiobooks", "Dune (1965)", "Chapter 1.mp3"), id3(map[string]string{"TPE1": "Frank Herbert"}))
	})
	books, _ := d.QueryItems(`WHERE type='Book' ORDER BY name`)
	if len(books) != 2 {
		t.Fatalf("books: %d", len(books))
	}
	comic, epub := books[0], books[1]
	if epub.Name != "Il nome della rosa" || epub.Artists[0] != "Umberto Eco" || epub.Overview != "Un giallo" || epub.Year != 1980 || epub.ImgPrimary == "" {
		t.Fatalf("epub: %+v", epub)
	}
	if comic.Name != "Captain Marvel Adventures v01 c120" || *comic.TrackNumber != 120 || *comic.DiscNumber != 1 || comic.ImgPrimary == "" {
		t.Fatalf("comic: %+v", comic)
	}
	ab, _ := d.QueryItems(`WHERE type='AudioBook'`)
	if len(ab) != 1 || ab[0].Name != "Dune" || ab[0].Year != 1965 || ab[0].Artists[0] != "Frank Herbert" {
		t.Fatalf("audiobook: %+v", ab)
	}
	ch, _ := d.QueryItems(`WHERE parent_id=? ORDER BY track_number`, ab[0].ID)
	if len(ch) != 2 || ch[0].Name != "Chapter 1" {
		t.Fatalf("chapters: %+v", ch)
	}
}
