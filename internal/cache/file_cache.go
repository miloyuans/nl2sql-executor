package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Entry struct {
	Key       string    `json:"key"`
	Summary   string    `json:"summary"`
	CSVPath   string    `json:"csv_path,omitempty"`
	ChartPath string    `json:"chart_path,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type FileCache struct {
	dir string
	ttl time.Duration
}

func NewFileCache(dir string, ttl time.Duration) *FileCache { return &FileCache{dir: dir, ttl: ttl} }

func HashKey(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		_, _ = h.Write([]byte(p))
		_, _ = h.Write([]byte("\x00"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (c *FileCache) Get(key string) (Entry, bool) {
	var e Entry
	if strings.TrimSpace(key) == "" {
		return e, false
	}
	b, err := os.ReadFile(c.path(key))
	if err != nil {
		return e, false
	}
	if err := json.Unmarshal(b, &e); err != nil {
		return e, false
	}
	if !e.ExpiresAt.IsZero() && time.Now().After(e.ExpiresAt) {
		_ = os.Remove(c.path(key))
		return Entry{}, false
	}
	return e, true
}

func (c *FileCache) Set(e Entry) error {
	if e.Key == "" {
		return nil
	}
	if err := os.MkdirAll(c.dir, 0755); err != nil {
		return err
	}
	e.CreatedAt = time.Now()
	if c.ttl > 0 {
		e.ExpiresAt = e.CreatedAt.Add(c.ttl)
	}
	b, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.path(e.Key), b, 0644)
}

func (c *FileCache) path(key string) string { return filepath.Join(c.dir, key+".json") }
