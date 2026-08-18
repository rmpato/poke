package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// DefaultInterval is how often poke may ask GitHub whether a newer release
// exists. Once a day is enough to hear about a release without the tool feeling
// like it is watching you.
const DefaultInterval = 24 * time.Hour

// checkTimeout bounds the background refresh. A slow network must never turn
// into a slow tool.
const checkTimeout = 8 * time.Second

// cacheFile is the name of the state file inside the poke data directory.
const cacheFile = "update-check.json"

// Cache remembers the last check so that ordinary runs read a file instead of
// reaching the network.
type Cache struct {
	CheckedAt time.Time `json:"checked_at"`
	Latest    string    `json:"latest"`
	URL       string    `json:"url,omitempty"`
}

// LoadCache reads the cached result. A missing or unreadable cache is not an
// error: it just means "no idea", which is the correct answer.
func LoadCache(dir string) Cache {
	var c Cache
	data, err := os.ReadFile(filepath.Join(dir, cacheFile))
	if err != nil {
		return c
	}
	_ = json.Unmarshal(data, &c)
	return c
}

// SaveCache records a check result.
func SaveCache(dir string, c Cache) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, cacheFile), data, 0o600)
}

// Stale reports whether the cached answer is old enough to be worth refreshing.
func (c Cache) Stale(interval time.Duration) bool {
	if c.CheckedAt.IsZero() {
		return true
	}
	return time.Since(c.CheckedAt) > interval
}

// Available returns the newer version the cache knows about, or "" when the
// current build is up to date. It never touches the network.
func (c Cache) Available(current string) string {
	if c.Latest == "" {
		return ""
	}
	if current == "dev" || current == "" {
		// A dev build has no meaningful version to compare, and nagging someone
		// who built from source would be noise.
		return ""
	}
	if CompareVersions(current, c.Latest) < 0 {
		return c.Latest
	}
	return ""
}

// Refresh asks GitHub for the latest release and updates the cache.
//
// The timestamp is written before the request so that a failing or slow network
// cannot cause every subsequent run to try again.
func Refresh(dir string, opts Options) (Cache, error) {
	c := Cache{CheckedAt: time.Now().UTC()}
	if err := SaveCache(dir, c); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return c, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()

	rel, err := Latest(ctx, opts)
	if err != nil {
		return c, err
	}

	c.Latest, c.URL = rel.Version(), rel.HTMLURL
	return c, SaveCache(dir, c)
}
