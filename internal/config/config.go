// Package config resolves where pogo keeps its data and how it behaves.
//
// Everything has a working default: pogo must run correctly on a machine with
// no config file at all, because the first thing a user does is type a curl
// command, not read documentation.
package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rmpato/poke/internal/history"
)

// App is the name pogo uses for its directories and its config file.
const App = "pogo"

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
	// Preferences is the whis block: theme, onboarding, dismissals.
	Preferences `yaml:",inline"`

	Capture Capture        `yaml:"capture,omitempty"`
	Redact  history.Policy `yaml:"redact,omitempty"`
	Update  Update         `yaml:"update,omitempty"`

	// APIs names and corrects what pogo inferred about the hosts in history:
	// which registrable domain is one API, and which environment a host is.
	APIs APIRegistry `yaml:"apis,omitempty"`

	// dir is where history lives; resolved, not serialized.
	dir string
}

// Capture controls what pogo records.
type Capture struct {
	// Disabled turns pogo into a pure curl passthrough.
	Disabled bool `yaml:"disabled,omitempty"`

	MaxResponseBody int64 `yaml:"max_response_body,omitempty"`
	MaxRequestBody  int64 `yaml:"max_request_body,omitempty"`

	// MaxEntries bounds the history so it stays useful rather than
	// overwhelming. Older entries are dropped at compaction; favorites are
	// always kept.
	MaxEntries int `yaml:"max_entries,omitempty"`

	// Curl overrides the binary pogo delegates to. Empty means "curl" from PATH.
	Curl string `yaml:"curl,omitempty"`
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

// DataDir resolves the history directory, honoring POGO_HOME and then XDG.
//
// The XDG layout is used on macOS as well as Linux. Developers who live in a
// terminal expect ~/.local/share, and a single documented path is easier to
// reason about (and to `rm -rf` when it holds secrets) than two.
func DataDir() string {
	if d := os.Getenv("POGO_HOME"); d != "" {
		return d
	}
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, App)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "." + App
	}
	return filepath.Join(home, ".local", "share", App)
}

// ConfigDir is the directory holding config.yaml and environments.yaml.
func ConfigDir() string {
	if f := os.Getenv("POGO_CONFIG"); f != "" {
		return filepath.Dir(f)
	}
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, App)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".config", App)
}

// File returns the path of the configuration file.
func File() string {
	if f := os.Getenv("POGO_CONFIG"); f != "" {
		return f
	}
	return filepath.Join(ConfigDir(), "config.yaml")
}

// EnvFile is the path of the environments file. It lives beside config.yaml but
// in its own file because it holds credentials and config.yaml does not.
func EnvFile() string {
	if f := os.Getenv("POGO_ENV_FILE"); f != "" {
		return f
	}
	return filepath.Join(ConfigDir(), "environments.yaml")
}

// Open loads the configuration into a store that can write it back. The TUI
// holds one of these so a preference persists on the keypress that changed it.
func Open() (*Store[Config], error) {
	store, err := OpenAt(File(), Default())
	if err != nil {
		return nil, err
	}
	// Normalized in memory only. Opening pogo must not create a config file for
	// someone who never asked for one; the first Update writes the whole
	// normalized value anyway.
	//
	// Note what is *not* done here: the environment overrides are not applied
	// to the stored value. POGO_REDACT=store for one command must not become
	// `redact: store` in the file the next time a preference is saved. Callers
	// read the effective configuration with WithEnv.
	cfg := store.value
	cfg.normalize()
	store.value = cfg
	return store, nil
}

// Load reads the configuration, falling back to defaults when the file is
// absent. A malformed file is an error: silently ignoring it could quietly
// disable a redaction rule the user believed was in force.
func Load() (Config, error) {
	store, err := OpenAt(File(), Default())
	if err != nil {
		return Default(), err
	}
	cfg := store.Current()
	cfg.normalize()
	return cfg.WithEnv(), nil
}

// WithEnv returns a copy with the POGO_* overrides applied, which is the
// configuration a command should actually run under. It is deliberately not
// what the store holds: an override is for this invocation, not for the file.
func (c Config) WithEnv() Config {
	c.applyEnv()
	return c
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
	if v := os.Getenv("POGO_CURL"); v != "" {
		c.Capture.Curl = v
	}
	if v := os.Getenv("POGO_NO_CAPTURE"); truthy(v) {
		c.Capture.Disabled = true
	}
	if v := os.Getenv("POGO_REDACT"); v != "" {
		switch strings.ToLower(v) {
		case "store":
			c.Redact.Mode = history.ModeStore
		case "display":
			c.Redact.Mode = history.ModeDisplay
		case "off", "none":
			c.Redact.Off = true
		}
	}
	if truthy(os.Getenv("POGO_NO_UPDATE_CHECK")) {
		c.Update.Disabled = true
	}
	if v := os.Getenv("POGO_MAX_BODY"); v != "" {
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
// used by `pogo config init` so users have a starting point to edit.
func (c Config) Save() error {
	store := &Store[Config]{path: File(), defaults: Default(), value: c}
	return store.Save()
}

// Update controls whether pogo may ask GitHub about newer releases.
//
// Checking is the only thing pogo does over the network that the user did not
// ask for, so it is bounded, cached, interactive-only, and switched off with a
// single field. Nothing is ever installed without an explicit confirmation.
type Update struct {
	// Disabled turns off the periodic check entirely.
	Disabled bool `yaml:"disabled,omitempty"`

	// IntervalHours is how often to ask. Zero means the default of 24 hours.
	IntervalHours int `yaml:"interval_hours,omitempty"`
}

// CheckInterval is how often pogo may look for a new release.
func (u Update) CheckInterval() time.Duration {
	if u.IntervalHours <= 0 {
		return 24 * time.Hour
	}
	return time.Duration(u.IntervalHours) * time.Hour
}
