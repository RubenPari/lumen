package metadata

import (
	"image"
	"os"
	"strings"

	"github.com/rwcarlsen/goexif/exif"
)

// PhotoInfo is technical information about an image.
type PhotoInfo struct {
	Width, Height int
	TakenAt       int64
	Orientation   int
	Camera        string
}

// ReadPhoto returns dimensions (orientation-corrected) and EXIF data.
func ReadPhoto(file string) PhotoInfo {
	var pi PhotoInfo
	f, err := os.Open(file)
	if err != nil {
		return pi
	}
	defer f.Close()
	if cfg, _, err := image.DecodeConfig(f); err == nil {
		pi.Width, pi.Height = cfg.Width, cfg.Height
	}
	if _, err := f.Seek(0, 0); err != nil {
		return pi
	}
	x, err := exif.Decode(f)
	if err != nil {
		return pi
	}
	if t, err := x.DateTime(); err == nil && t.Year() > 1900 {
		pi.TakenAt = t.Unix()
	}
	if tag, err := x.Get(exif.Orientation); err == nil {
		if v, err := tag.Int(0); err == nil {
			pi.Orientation = v
		}
	}
	var parts []string
	for _, name := range []exif.FieldName{exif.Make, exif.Model} {
		if tag, err := x.Get(name); err == nil {
			if s, err := tag.StringVal(); err == nil {
				parts = append(parts, strings.TrimSpace(strings.Trim(s, "\x00")))
			}
		}
	}
	pi.Camera = strings.Join(parts, " ")
	if pi.Orientation >= 5 && pi.Orientation <= 8 {
		pi.Width, pi.Height = pi.Height, pi.Width
	}
	return pi
}

// Orientation reads only the EXIF orientation of a JPEG (1 when unknown).
func Orientation(file string) int {
	f, err := os.Open(file)
	if err != nil {
		return 1
	}
	defer f.Close()
	x, err := exif.Decode(f)
	if err != nil {
		return 1
	}
	tag, err := x.Get(exif.Orientation)
	if err != nil {
		return 1
	}
	v, err := tag.Int(0)
	if err != nil || v < 1 || v > 8 {
		return 1
	}
	return v
}
