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
	"sync"

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
	b := img.Bounds()
	if b.Dx() <= width {
		return src, nil
	}
	height := b.Dy() * width / b.Dx()
	out := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(out, out.Bounds(), img, b, draw.Src, nil)
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
