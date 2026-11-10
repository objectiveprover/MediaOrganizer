package tmdb

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"time"
)

// Cache stores raw API responses on disk, one file per request.
type Cache struct {
	Dir string
	TTL time.Duration
}

// NewCache creates a cache in dir. Returns nil if the directory can't be created,
// which disables caching rather than failing.
func NewCache(dir string, ttl time.Duration) *Cache {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil
	}
	return &Cache{Dir: dir, TTL: ttl}
}

// DefaultCacheDir returns ~/.cache/renamer/tmdb (or the OS equivalent).
func DefaultCacheDir() string {
	d, err := os.UserCacheDir()
	if err != nil {
		d = os.TempDir()
	}
	return filepath.Join(d, "renamer", "tmdb")
}

func (c *Cache) path(key string) string {
	h := sha256.Sum256([]byte(key))
	s := hex.EncodeToString(h[:])
	return filepath.Join(c.Dir, s[:2], s+".json")
}

// Get returns a cached body if present and fresh.
func (c *Cache) Get(key string) ([]byte, bool) {
	if c == nil {
		return nil, false
	}
	p := c.path(key)
	st, err := os.Stat(p)
	if err != nil || time.Since(st.ModTime()) > c.TTL {
		return nil, false
	}
	b, err := os.ReadFile(p)
	return b, err == nil
}

// Put stores body under key. Errors are ignored: the cache is best-effort.
func (c *Cache) Put(key string, body []byte) {
	if c == nil {
		return
	}
	p := c.path(key)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, p)
}
