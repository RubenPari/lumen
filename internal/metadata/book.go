package metadata

import (
	"archive/zip"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nwaples/rardecode/v2"
)

// BookMeta is metadata read from an e-book file.
type BookMeta struct {
	Title       string
	Authors     []string
	Description string
	Publisher   string
	Language    string
	Date        string
	Subjects    []string
	Series      string
	SeriesIndex string
	Cover       []byte
	CoverExt    string
}

type opfPackage struct {
	Metadata struct {
		Title       []string `xml:"title"`
		Creator     []string `xml:"creator"`
		Description string   `xml:"description"`
		Publisher   string   `xml:"publisher"`
		Language    string   `xml:"language"`
		Date        string   `xml:"date"`
		Subject     []string `xml:"subject"`
		Meta        []struct {
			Name     string `xml:"name,attr"`
			Content  string `xml:"content,attr"`
			Property string `xml:"property,attr"`
			Value    string `xml:",chardata"`
		} `xml:"meta"`
	} `xml:"metadata"`
	Manifest struct {
		Items []struct {
			ID         string `xml:"id,attr"`
			Href       string `xml:"href,attr"`
			MediaType  string `xml:"media-type,attr"`
			Properties string `xml:"properties,attr"`
		} `xml:"item"`
	} `xml:"manifest"`
}

func zipFile(zr *zip.Reader, name string) ([]byte, error) {
	for _, f := range zr.File {
		if f.Name == name || strings.EqualFold(f.Name, name) {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(io.LimitReader(rc, 64<<20))
		}
	}
	return nil, os.ErrNotExist
}

var htmlTag = strings.NewReplacer("<p>", "", "</p>", "\n\n", "<br/>", "\n", "<br>", "\n", "<br />", "\n")

func stripHTML(s string) string {
	s = htmlTag.Replace(s)
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == '<':
			in = true
		case r == '>':
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// ReadEPUB parses the OPF package of an EPUB file.
func ReadEPUB(file string) (*BookMeta, error) {
	zr, err := zip.OpenReader(file)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	container, err := zipFile(&zr.Reader, "META-INF/container.xml")
	if err != nil {
		return nil, err
	}
	var c struct {
		Rootfiles []struct {
			FullPath string `xml:"full-path,attr"`
		} `xml:"rootfiles>rootfile"`
	}
	if err := xml.Unmarshal(container, &c); err != nil || len(c.Rootfiles) == 0 {
		return nil, errors.New("invalid container.xml")
	}
	opfPath := c.Rootfiles[0].FullPath
	data, err := zipFile(&zr.Reader, opfPath)
	if err != nil {
		return nil, err
	}
	var p opfPackage
	if err := xml.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	m := &BookMeta{Description: stripHTML(p.Metadata.Description), Publisher: p.Metadata.Publisher, Language: p.Metadata.Language, Date: p.Metadata.Date}
	if len(p.Metadata.Title) > 0 {
		m.Title = strings.TrimSpace(p.Metadata.Title[0])
	}
	for _, a := range p.Metadata.Creator {
		if a = strings.TrimSpace(a); a != "" {
			m.Authors = append(m.Authors, a)
		}
	}
	m.Subjects = p.Metadata.Subject
	coverID := ""
	for _, meta := range p.Metadata.Meta {
		switch {
		case meta.Name == "cover":
			coverID = meta.Content
		case meta.Name == "calibre:series":
			m.Series = meta.Content
		case meta.Name == "calibre:series_index":
			m.SeriesIndex = meta.Content
		case meta.Property == "belongs-to-collection":
			m.Series = strings.TrimSpace(meta.Value)
		case meta.Property == "group-position":
			m.SeriesIndex = strings.TrimSpace(meta.Value)
		}
	}
	coverHref := ""
	for _, it := range p.Manifest.Items {
		if strings.Contains(it.Properties, "cover-image") || (coverID != "" && it.ID == coverID) {
			coverHref = it.Href
			break
		}
	}
	if coverHref == "" {
		for _, it := range p.Manifest.Items {
			if strings.HasPrefix(it.MediaType, "image/") && strings.Contains(strings.ToLower(it.ID+it.Href), "cover") {
				coverHref = it.Href
				break
			}
		}
	}
	if coverHref != "" {
		full := path.Join(path.Dir(opfPath), coverHref)
		if b, err := zipFile(&zr.Reader, full); err == nil {
			m.Cover, m.CoverExt = b, strings.ToLower(path.Ext(full))
		}
	}
	return m, nil
}

func isImageName(n string) bool {
	switch strings.ToLower(path.Ext(n)) {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
		return !strings.HasPrefix(path.Base(n), ".") && !strings.Contains(n, "__MACOSX")
	}
	return false
}

// NaturalLess sorts "page2" before "page10".
func NaturalLess(a, b string) bool {
	a, b = strings.ToLower(a), strings.ToLower(b)
	for a != "" && b != "" {
		da, db := digitsPrefix(a), digitsPrefix(b)
		if da != "" && db != "" {
			na, nb := strings.TrimLeft(da, "0"), strings.TrimLeft(db, "0")
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			a, b = a[len(da):], b[len(db):]
			continue
		}
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		a, b = a[1:], b[1:]
	}
	return len(a) < len(b)
}

func digitsPrefix(s string) string {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[:i]
}

// ComicPages lists the image pages of a CBZ/CBR archive in reading order.
func ComicPages(file string) ([]string, error) {
	var names []string
	switch strings.ToLower(filepath.Ext(file)) {
	case ".cbz":
		zr, err := zip.OpenReader(file)
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		for _, f := range zr.File {
			if !f.FileInfo().IsDir() && isImageName(f.Name) {
				names = append(names, f.Name)
			}
		}
	case ".cbr":
		rr, err := rardecode.OpenReader(file)
		if err != nil {
			return nil, err
		}
		defer rr.Close()
		for {
			h, err := rr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			if !h.IsDir && isImageName(h.Name) {
				names = append(names, h.Name)
			}
		}
	default:
		return nil, errors.New("not a comic archive")
	}
	sort.Slice(names, func(i, j int) bool { return NaturalLess(names[i], names[j]) })
	return names, nil
}

// ComicPage returns the bytes of one page of a comic archive.
func ComicPage(file, name string) ([]byte, error) {
	switch strings.ToLower(filepath.Ext(file)) {
	case ".cbz":
		zr, err := zip.OpenReader(file)
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		return zipFile(&zr.Reader, name)
	case ".cbr":
		rr, err := rardecode.OpenReader(file)
		if err != nil {
			return nil, err
		}
		defer rr.Close()
		for {
			h, err := rr.Next()
			if err != nil {
				return nil, err
			}
			if h.Name == name {
				return io.ReadAll(io.LimitReader(rr, 64<<20))
			}
		}
	}
	return nil, errors.New("not a comic archive")
}

// ReadBook returns metadata (and cover) for any supported book format.
func ReadBook(file string) *BookMeta {
	switch strings.ToLower(filepath.Ext(file)) {
	case ".epub":
		if m, err := ReadEPUB(file); err == nil {
			return m
		}
	case ".cbz", ".cbr":
		if pages, err := ComicPages(file); err == nil && len(pages) > 0 {
			if b, err := ComicPage(file, pages[0]); err == nil {
				return &BookMeta{Cover: b, CoverExt: strings.ToLower(path.Ext(pages[0]))}
			}
		}
	}
	return &BookMeta{}
}

// ParseDateYear returns the year of an ISO-ish date.
func ParseDateYear(s string) int {
	s = strings.TrimSpace(s)
	if len(s) < 4 {
		return 0
	}
	if t, err := time.Parse("2006", s[:4]); err == nil {
		return t.Year()
	}
	return 0
}
