package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rmpato/poke/internal/history"
)

// An environment variable overrides the file for one invocation. It must never
// become the file: saving a preference from the UI while POGO_REDACT=store is
// exported would otherwise silently make that the permanent setting.
func TestEnvOverridesAreNotPersisted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	t.Setenv("POGO_CONFIG", path)
	t.Setenv("POGO_HOME", dir)
	t.Setenv("POGO_REDACT", "store")
	t.Setenv("POGO_NO_CAPTURE", "1")

	store, err := Open()
	if err != nil {
		t.Fatal(err)
	}

	// What a command runs under: the override applies.
	if got := store.Current().WithEnv(); got.Redact.Mode != history.ModeStore || !got.Capture.Disabled {
		t.Errorf("the override did not reach the effective config: %+v", got.Redact)
	}

	// What gets written: it does not.
	if err := store.Update(func(c *Config) { c.Theme = "sunset" }); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, leaked := range []string{"store", "disabled"} {
		if strings.Contains(string(data), leaked) {
			t.Errorf("the config file records an environment override:\n%s", data)
		}
	}

	// And the preference that was actually changed did get written.
	if !strings.Contains(string(data), "sunset") {
		t.Errorf("the theme was not saved:\n%s", data)
	}
}
