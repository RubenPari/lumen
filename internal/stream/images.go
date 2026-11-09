package stream

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/rubenpari/lumen/internal/metadata"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// ImageCache resizes images on demand and caches the results on disk.
type ImageCache struct {
	Dir      string
	mu       sync.Mutex
	inflight map[string]*sync.Mutex
}

// NewImageCache creates a cache in dir.
func NewImageCache(dir string) *ImageCache {
	_ = os.MkdirAll(dir, 0o755)
	return &ImageCache{Dir: dir, inflight: map[string]*sync.Mutex{}}
}

func (c *ImageCache) lock(key string) func() {
	c.mu.Lock()
	m, ok := c.inflight[key]
	if !ok {
		m = &sync.Mutex{}
		c.inflight[key] = m
	}
	c.mu.Unlock()
	m.Lock()
	return m.Unlock
}

// Resized returns the path of src resized to the given max width (0 = original).
func (c *ImageCache) Resized(src string, width int) (string, error) {
	fi, err := os.Stat(src)
	if err != nil {
		return "", err
	}
	if width <= 0 {
		return src, nil
	}
	h := sha1.Sum([]byte(fmt.Sprintf("%s|%d|%d|%d", src, fi.ModTime().UnixNano(), fi.Size(), width)))
	key := hex.EncodeToString(h[:])
	dst := filepath.Join(c.Dir, key[:2], key+".jpg")
	if _, err := os.Stat(dst); err == nil {
		return dst, nil
	}
	unlock := c.lock(key)
	defer unlock()
	if _, err := os.Stat(dst); err == nil {
		return dst, nil
	}
	f, err := os.Open(src)
	if err != nil {
		return "", err
	}
	img, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		return src, nil // unknown format: serve the original
	}
	orient := 1
	if ext := strings.ToLower(filepath.Ext(src)); ext == ".jpg" || ext == ".jpeg" {
		orient = metadata.Orientation(src)
	}
	b := img.Bounds()
	if b.Dx() <= width && orient == 1 {
		return src, nil
	}
	if width > b.Dx() {
		width = b.Dx()
	}
	height := b.Dy() * width / b.Dx()
	var out image.Image = image.NewRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(out.(*image.RGBA), out.Bounds(), img, b, draw.Src, nil)
	out = applyOrientation(out, orient)
	_ = os.MkdirAll(filepath.Dir(dst), 0o755)
	tmp := dst + ".tmp"
	w, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	if err := jpeg.Encode(w, out, &jpeg.Options{Quality: 86}); err != nil {
		w.Close()
		os.Remove(tmp)
		return "", err
	}
	w.Close()
	return dst, os.Rename(tmp, dst)
}

// applyOrientation rotates/flips an image according to its EXIF orientation.
func applyOrientation(img image.Image, o int) image.Image {
	if o <= 1 || o > 8 {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	ow, oh := w, h
	if o >= 5 {
		ow, oh = h, w
	}
	out := image.NewRGBA(image.Rect(0, 0, ow, oh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var nx, ny int
			switch o {
			case 2:
				nx, ny = w-1-x, y
			case 3:
				nx, ny = w-1-x, h-1-y
			case 4:
				nx, ny = x, h-1-y
			case 5:
				nx, ny = y, x
			case 6:
				nx, ny = h-1-y, x
			case 7:
				nx, ny = h-1-y, w-1-x
			case 8:
				nx, ny = y, w-1-x
			}
			out.Set(nx, ny, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return out
}
