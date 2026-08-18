// Package config resolves where poke keeps its data and how it behaves.
//
// Everything has a working default: poke must run correctly on a machine with
// no config file at all, because the first thing a user does is type a curl
// command, not read documentation.
package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rmpato/poke/internal/history"
)

// Defaults for payload capture. These are ceilings, not targets: exceeding them
// truncates the stored copy and marks it, and never affects what the user sees
// in the terminal or what curl actually transferred.
const (
	DefaultMaxResponseBody = 2 << 20 // 2 MiB
	DefaultMaxRequestBody  = 1 << 20 // 1 MiB
	DefaultMaxStderr       = 64 << 10
	DefaultMaxEntries      = 5000
)

// Config is the on-disk configuration, all fields optional.
type Config struct {
	Capture Capture        `json:"capture"`
	Redact  history.Policy `json:"redact"`
	Update  Update         `json:"update"`

	// dir is where history lives; resolved, not serialized.
	dir string
}

// Capture controls what poke records.
type Capture struct {
	// Disabled turns poke into a pure curl passthrough.
	Disabled bool `json:"disabled,omitempty"`

	MaxResponseBody int64 `json:"max_response_body,omitempty"`
	MaxRequestBody  int64 `json:"max_request_body,omitempty"`

	// MaxEntries bounds the history so it stays useful rather than
	// overwhelming. Older entries are dropped at compaction; favorites are
	// always kept.
	MaxEntries int `json:"max_entries,omitempty"`

	// Curl overrides the binary poke delegates to. Empty means "curl" from PATH.
	Curl string `json:"curl,omitempty"`
}

// Default returns the configuration used when no file exists.
func Default() Config {
	return Config{
		Capture: Capture{
			MaxResponseBody: DefaultMaxResponseBody,
			MaxRequestBody:  DefaultMaxRequestBody,
			MaxEntries:      DefaultMaxEntries,
		},
		Redact: history.DefaultPolicy(),
	}
}

// Dir is the directory holding history.jsonl and the blob store.
func (c Config) Dir() string { return c.dir }

// HistoryFile is the append-only log of captured requests.
func (c Config) HistoryFile() string { return filepath.Join(c.dir, "history.jsonl") }

// BlobDir holds request and response payloads, kept out of the log so that
// loading history does not mean reading every byte ever transferred.
func (c Config) BlobDir() string { return filepath.Join(c.dir, "blobs") }

// DataDir resolves the history directory, honoring POKE_HOME and then XDG.
//
// The XDG layout is used on macOS as well as Linux. Developers who live in a
// terminal expect ~/.local/share, and a single documented path is easier to
// reason about (and to `rm -rf` when it holds secrets) than two.
func DataDir() string {
	if d := os.Getenv("POKE_HOME"); d != "" {
		return d
	}
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "poke")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".poke"
	}
	return filepath.Join(home, ".local", "share", "poke")
}

// File returns the path of the configuration file.
func File() string {
	if f := os.Getenv("POKE_CONFIG"); f != "" {
		return f
	}
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "poke", "config.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "config.json"
	}
	return filepath.Join(home, ".config", "poke", "config.json")
}

// Load reads the configuration, falling back to defaults when the file is
// absent. A malformed file is an error: silently ignoring it could quietly
// disable a redaction rule the user believed was in force.
func Load() (Config, error) {
	cfg := Default()
	cfg.dir = DataDir()

	data, err := os.ReadFile(File())
	switch {
	case errors.Is(err, fs.ErrNotExist):
		cfg.applyEnv()
		return cfg, nil
	case err != nil:
		return cfg, err
	}

	// Decode over the defaults so omitted fields keep their default value.
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	cfg.normalize()
	cfg.applyEnv()
	return cfg, nil
}

func (c *Config) normalize() {
	d := Default()
	if c.Capture.MaxResponseBody <= 0 {
		c.Capture.MaxResponseBody = d.Capture.MaxResponseBody
	}
	if c.Capture.MaxRequestBody <= 0 {
		c.Capture.MaxRequestBody = d.Capture.MaxRequestBody
	}
	if c.Capture.MaxEntries < 0 {
		c.Capture.MaxEntries = 0 // 0 means unlimited
	}
	if c.Redact.Mode == "" {
		c.Redact.Mode = history.ModeDisplay
	}
	c.dir = DataDir()
}

// applyEnv lets one-off invocations override the file without editing it.
func (c *Config) applyEnv() {
	if v := os.Getenv("POKE_CURL"); v != "" {
		c.Capture.Curl = v
	}
	if v := os.Getenv("POKE_NO_CAPTURE"); truthy(v) {
		c.Capture.Disabled = true
	}
	if v := os.Getenv("POKE_REDACT"); v != "" {
		switch strings.ToLower(v) {
		case "store":
			c.Redact.Mode = history.ModeStore
		case "display":
			c.Redact.Mode = history.ModeDisplay
		case "off", "none":
			c.Redact.Off = true
		}
	}
	if truthy(os.Getenv("POKE_NO_UPDATE_CHECK")) {
		c.Update.Disabled = true
	}
	if v := os.Getenv("POKE_MAX_BODY"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			c.Capture.MaxResponseBody = n
			c.Capture.MaxRequestBody = n
		}
	}
}

func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "0", "false", "no", "off":
		return false
	}
	return true
}

// ForDir returns a configuration rooted at an explicit directory. Tests use it
// to stay away from the developer's real history.
func ForDir(dir string) Config {
	c := Default()
	c.dir = dir
	return c
}

// Save writes the configuration, creating parent directories as needed. It is
// used by `pogo --init-config` so users have a commented starting point.
func (c Config) Save() error {
	path := File()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// Update controls whether poke may ask GitHub about newer releases.
//
// Checking is the only thing poke does over the network that the user did not
// ask for, so it is bounded, cached, interactive-only, and switched off with a
// single field. Nothing is ever installed without an explicit confirmation.
type Update struct {
	// Disabled turns off the periodic check entirely.
	Disabled bool `json:"disabled,omitempty"`

	// IntervalHours is how often to ask. Zero means the default of 24 hours.
	IntervalHours int `json:"interval_hours,omitempty"`
}

// CheckInterval is how often poke may look for a new release.
func (u Update) CheckInterval() time.Duration {
	if u.IntervalHours <= 0 {
		return 24 * time.Hour
	}
	return time.Duration(u.IntervalHours) * time.Hour
}
