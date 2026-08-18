package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rmpato/poke/internal/history"
)

func TestDefaultsAreUsable(t *testing.T) {
	c := Default()
	if c.Capture.MaxResponseBody <= 0 || c.Capture.MaxRequestBody <= 0 {
		t.Error("body caps must have working defaults")
	}
	if c.Capture.Disabled {
		t.Error("capture should be on by default; recording is the point")
	}
	if c.Redact.Mode != history.ModeDisplay {
		t.Errorf("default redaction mode = %q, want display", c.Redact.Mode)
	}
}

func TestDataDirPrecedence(t *testing.T) {
	t.Setenv("POKE_HOME", "/custom/poke")
	if got := DataDir(); got != "/custom/poke" {
		t.Errorf("POKE_HOME should win, got %q", got)
	}

	t.Setenv("POKE_HOME", "")
	t.Setenv("XDG_DATA_HOME", "/xdg")
	if got := DataDir(); got != filepath.Join("/xdg", "poke") {
		t.Errorf("XDG_DATA_HOME should be honoured, got %q", got)
	}

	t.Setenv("XDG_DATA_HOME", "")
	home, _ := os.UserHomeDir()
	if got := DataDir(); got != filepath.Join(home, ".local", "share", "poke") {
		t.Errorf("fallback = %q", got)
	}
}

func TestLoadWithoutFileUsesDefaults(t *testing.T) {
	t.Setenv("POKE_CONFIG", filepath.Join(t.TempDir(), "nope.json"))
	t.Setenv("POKE_HOME", t.TempDir())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("a missing config file must not be an error: %v", err)
	}
	if cfg.Capture.MaxResponseBody != DefaultMaxResponseBody {
		t.Error("defaults were not applied")
	}
}

func TestLoadPartialFileKeepsOtherDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"capture":{"max_response_body":123}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("POKE_CONFIG", path)
	t.Setenv("POKE_HOME", dir)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Capture.MaxResponseBody != 123 {
		t.Errorf("configured value = %d, want 123", cfg.Capture.MaxResponseBody)
	}
	if cfg.Capture.MaxRequestBody != DefaultMaxRequestBody {
		t.Error("an unset field should keep its default")
	}
	if cfg.Redact.Mode != history.ModeDisplay {
		t.Error("an unset redaction mode should default rather than become empty")
	}
}

// A malformed config could silently disable a redaction rule the user believes
// is protecting them, so it must fail loudly.
func TestLoadRejectsMalformedConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"capture": `), 0o600)
	t.Setenv("POKE_CONFIG", path)
	t.Setenv("POKE_HOME", dir)

	if _, err := Load(); err == nil {
		t.Error("a malformed config file should be reported, not ignored")
	}
}

func TestEnvironmentOverrides(t *testing.T) {
	t.Setenv("POKE_CONFIG", filepath.Join(t.TempDir(), "none.json"))
	t.Setenv("POKE_HOME", t.TempDir())
	t.Setenv("POKE_CURL", "/opt/curl")
	t.Setenv("POKE_NO_CAPTURE", "1")
	t.Setenv("POKE_REDACT", "store")
	t.Setenv("POKE_MAX_BODY", "4096")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Capture.Curl != "/opt/curl" {
		t.Errorf("POKE_CURL = %q", cfg.Capture.Curl)
	}
	if !cfg.Capture.Disabled {
		t.Error("POKE_NO_CAPTURE should disable capture")
	}
	if cfg.Redact.Mode != history.ModeStore {
		t.Errorf("POKE_REDACT = %q", cfg.Redact.Mode)
	}
	if cfg.Capture.MaxResponseBody != 4096 {
		t.Errorf("POKE_MAX_BODY = %d", cfg.Capture.MaxResponseBody)
	}
}

func TestPokeNoCaptureIgnoresFalsyValues(t *testing.T) {
	t.Setenv("POKE_CONFIG", filepath.Join(t.TempDir(), "none.json"))
	t.Setenv("POKE_HOME", t.TempDir())

	for _, v := range []string{"0", "false", "no", "off", ""} {
		t.Setenv("POKE_NO_CAPTURE", v)
		cfg, _ := Load()
		if cfg.Capture.Disabled {
			t.Errorf("POKE_NO_CAPTURE=%q should not disable capture", v)
		}
	}
	for _, v := range []string{"1", "true", "yes"} {
		t.Setenv("POKE_NO_CAPTURE", v)
		cfg, _ := Load()
		if !cfg.Capture.Disabled {
			t.Errorf("POKE_NO_CAPTURE=%q should disable capture", v)
		}
	}
}

func TestSaveWritesPrivateFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "config.json")
	t.Setenv("POKE_CONFIG", path)
	t.Setenv("POKE_HOME", dir)

	if err := Default().Save(); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// The config can name sensitive header patterns; keep it to the owner.
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("config mode = %o, want 600", perm)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("a config poke wrote must be one poke can read: %v", err)
	}
	if cfg.Capture.MaxResponseBody != DefaultMaxResponseBody {
		t.Error("saved config did not round trip")
	}
}

func TestPathsHangOffDataDir(t *testing.T) {
	c := ForDir("/tmp/poke-test")
	if c.HistoryFile() != "/tmp/poke-test/history.jsonl" {
		t.Errorf("HistoryFile = %q", c.HistoryFile())
	}
	if c.BlobDir() != "/tmp/poke-test/blobs" {
		t.Errorf("BlobDir = %q", c.BlobDir())
	}
}
