package config

// Store is the whis config store (SYSTEM_DESIGN.md §11), copied into pogo per
// the design system's "own your copy" rule. The rules it exists to enforce:
//
//   - A mutation persists on the same keypress that made it. There is no
//     "unsaved changes" state, because there is no such state to get wrong.
//   - A write that fails leaves the in-memory value untouched, so the model
//     and the disk can never disagree.
//   - An older or hand-edited file always loads. Unknown fields are ignored
//     and missing ones fall back to defaults.
//
// pogo keeps its own path resolution (see paths.go) rather than whis's
// os.UserConfigDir: XDG on macOS as well as Linux is a deliberate choice for a
// tool whose users live in a terminal.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Preferences is the block every whis app has, whatever else it stores.
// Keep it additive-only across versions: a field that disappears takes a
// user's setting with it, and a renamed field silently resets one.
type Preferences struct {
	// Theme is the active theme name, normalised through NormalizeTheme at
	// every read site so a hand-edited value degrades instead of erroring.
	Theme string `yaml:"theme,omitempty"`
	// OnboardingSeen suppresses the first-run walkthrough. Set it the moment
	// the user dismisses onboarding, not when they finish reading it.
	OnboardingSeen bool `yaml:"onboarding_seen,omitempty"`
	// Dismissed holds ids the user has hidden — notices, tips, rows. Storing
	// dismissals rather than a "show tips" boolean means a new tip is still
	// shown to someone who dismissed the last one.
	Dismissed []string `yaml:"dismissed,omitempty"`
}

// IsDismissed reports whether id has been hidden.
func (p Preferences) IsDismissed(id string) bool {
	for _, dismissed := range p.Dismissed {
		if dismissed == id {
			return true
		}
	}
	return false
}

// Dismiss adds id, ignoring duplicates.
func (p *Preferences) Dismiss(id string) {
	if id == "" || p.IsDismissed(id) {
		return
	}
	p.Dismissed = append(p.Dismissed, id)
}

// Restore removes id from the dismissed list.
func (p *Preferences) Restore(id string) {
	kept := p.Dismissed[:0]
	for _, dismissed := range p.Dismissed {
		if dismissed != id {
			kept = append(kept, dismissed)
		}
	}
	p.Dismissed = kept
}

// Store holds pogo's config in memory and keeps it in step with disk.
type Store[T any] struct {
	path     string
	defaults T
	value    T
}

// OpenAt loads a config file, falling back to defaults for a missing file or
// any missing field. A malformed file is the one case that errors — silently
// discarding a config a user hand-edited badly would lose their settings
// without telling them, and in pogo it could quietly disable a redaction rule
// they believe is protecting them.
func OpenAt[T any](path string, defaults T) (*Store[T], error) {
	store := &Store[T]{path: path, defaults: defaults, value: defaults}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil // first run
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	// Start from defaults so a field the file omits keeps its default rather
	// than becoming the zero value.
	value := defaults
	if err := yaml.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	store.value = value
	return store, nil
}

// Path returns the file this store reads and writes.
func (s *Store[T]) Path() string { return s.path }

// Current returns the in-memory config. It is always what was last
// successfully written, or the defaults.
func (s *Store[T]) Current() T { return s.value }

// Defaults returns the values this store falls back to.
func (s *Store[T]) Defaults() T { return s.defaults }

// Update applies mutate to a copy, writes it, and only then adopts it.
// A failed write leaves Current() exactly as it was — that is the rollback
// §11 asks for, and it is why mutators should go through here rather than
// assigning fields directly.
func (s *Store[T]) Update(mutate func(*T)) error {
	next := s.value
	mutate(&next)
	if err := s.write(next); err != nil {
		return err
	}
	s.value = next
	return nil
}

// Save writes the current in-memory value, creating the file if needed.
func (s *Store[T]) Save() error { return s.write(s.value) }

// Reload re-reads from disk, for a file edited by hand while pogo is running.
func (s *Store[T]) Reload() error {
	fresh, err := OpenAt(s.path, s.defaults)
	if err != nil {
		return err
	}
	s.value = fresh.value
	return nil
}

// write serialises atomically: a temp file in the same directory followed by
// a rename, so a crash mid-write cannot leave a half-written config that the
// next start refuses to parse.
func (s *Store[T]) write(value T) error {
	data, err := yaml.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	temp, err := os.CreateTemp(dir, ".config-*.yaml")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tempName := temp.Name()
	defer func() { _ = os.Remove(tempName) }() // no-op once the rename succeeds

	header := "# pogo configuration. Hand edits are kept; unknown keys are ignored.\n"
	if _, err := temp.WriteString(header); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write %s: %w", tempName, err)
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write %s: %w", tempName, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tempName, err)
	}
	if err := os.Chmod(tempName, 0o600); err != nil {
		return fmt.Errorf("chmod %s: %w", tempName, err)
	}
	if err := os.Rename(tempName, s.path); err != nil {
		return fmt.Errorf("replace %s: %w", s.path, err)
	}
	return nil
}

// Normalize folds a free-text preference value to one of allowed, returning
// fallback when it matches nothing. Run every read of a string preference
// through this: a legacy or mistyped value should degrade to something
// sensible, never stop the app.
func Normalize(value string, allowed []string, fallback string) string {
	cleaned := strings.ToLower(strings.TrimSpace(value))
	for _, candidate := range allowed {
		if cleaned == strings.ToLower(candidate) {
			return candidate
		}
	}
	return fallback
}
