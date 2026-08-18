package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// The registry is the single source of truth for what pogo can do, so it has to
// be well formed: anything malformed here shows up as a blank row in the
// palette or a missing line in the help reference.
func TestCommandRegistryIsWellFormed(t *testing.T) {
	m := newTestModel(t, sampleEntries()...)

	seen := map[string]bool{}
	for _, c := range m.commands() {
		if c.id == "" || c.title == "" {
			t.Errorf("command %+v needs an id and a title", c)
		}
		if seen[c.id] {
			t.Errorf("duplicate command id %q", c.id)
		}
		seen[c.id] = true

		if c.group == "" {
			t.Errorf("command %q has no group, so it cannot appear in help", c.id)
		}
		// A palette entry that does nothing is worse than no entry at all.
		if !c.motion && c.run == nil {
			t.Errorf("command %q is offered in the palette but does nothing", c.id)
		}
		// A motion is documentation only; it must not claim to be runnable.
		if c.motion && c.run != nil {
			t.Errorf("motion %q should not have a run function", c.id)
		}
	}

	for _, want := range []string{"replay", "edit", "diff", "search", "help", "quit"} {
		if !seen[want] {
			t.Errorf("registry is missing %q", want)
		}
	}
}

// Every action reachable by key must also be reachable by name, or the palette
// is not actually the answer to "what can this do?".
func TestPaletteCoversTheKeyedActions(t *testing.T) {
	m := newTestModel(t, sampleEntries()...)

	inPalette := map[string]bool{}
	for _, c := range m.paletteItems() {
		inPalette[c.id] = true
	}
	for _, want := range []string{"replay", "edit", "copy", "diff", "delete", "star", "collection", "env"} {
		if !inPalette[want] {
			t.Errorf("%q has a key but cannot be found in the palette", want)
		}
	}
}

func TestFuzzyScore(t *testing.T) {
	tests := []struct {
		text, query string
		match       bool
	}{
		{"Replay request", "rep", true},
		{"Replay request", "rr", true}, // subsequence across words
		{"Replay request", "xyz", false},
		{"Copy…", "cop", true},
		{"Compare with…", "comp", true},
		{"Add to collection…", "coll", true},
		{"Anything", "", true},
	}
	for _, tt := range tests {
		if _, ok := fuzzyScore(tt.text, tt.query); ok != tt.match {
			t.Errorf("fuzzyScore(%q, %q) matched=%v, want %v", tt.text, tt.query, ok, tt.match)
		}
	}

	// The more direct match should rank higher: typing "comp" wants Compare.
	compare, _ := fuzzyScore("Compare with…", "comp")
	copyCmd, _ := fuzzyScore("Copy…", "comp")
	if compare <= copyCmd {
		t.Errorf("Compare scored %d, Copy %d; the closer match should win", compare, copyCmd)
	}
}

func TestPaletteOpensFiltersAndRuns(t *testing.T) {
	m := newTestModel(t, sampleEntries()...)

	m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	if m.overlay != overlayPalette {
		t.Fatal("ctrl+k should open the palette")
	}
	if !strings.Contains(m.View(), "COMMANDS") {
		t.Error("the palette should be visible")
	}

	// Typing narrows it.
	for _, r := range "edit" {
		press(m, string(r))
	}
	items := m.filterCommands(m.paletteInput.Value())
	if len(items) == 0 || items[0].id != "edit" {
		t.Fatalf("typing \"edit\" should rank the edit command first, got %+v", items)
	}

	// Enter runs the selected command — here, opening the editor.
	press(m, "enter")
	if m.overlay != overlayNone {
		t.Error("running a command should close the palette")
	}
	if m.screen != screenEdit {
		t.Errorf("screen = %v, want the editor", m.screen)
	}
}

func TestPaletteShowsKeysSoItTeaches(t *testing.T) {
	m := newTestModel(t, sampleEntries()...)
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})

	view := m.View()
	// The point of the palette is that you learn the shortcut by using it.
	for _, want := range []string{"Replay request", "Compare with…"} {
		if !strings.Contains(view, want) {
			t.Errorf("palette does not list %q", want)
		}
	}
	if !strings.Contains(view, "r") || !strings.Contains(view, "d") {
		t.Error("palette rows should carry their keyboard shortcuts")
	}
}

func TestPaletteEscapeCloses(t *testing.T) {
	m := newTestModel(t, sampleEntries()...)
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	press(m, "esc")
	if m.overlay != overlayNone {
		t.Error("esc should close the palette")
	}
}

// The palette still lists commands that cannot run right now, marked, because
// hiding them would mean the feature is invisible exactly when someone is
// looking for it.
func TestPaletteKeepsUnavailableCommandsVisible(t *testing.T) {
	m := newTestModel(t) // no entries, so nothing is selected

	items := m.filterCommands("replay")
	if len(items) == 0 {
		t.Fatal("replay should still be listed with an empty history")
	}
	if items[0].available(m) {
		t.Error("replay should report itself unavailable with nothing selected")
	}

	m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	press(m, "enter")
	if m.screen == screenEdit {
		t.Error("an unavailable command must not run")
	}
}

// The footer is where a new user learns the palette exists.
func TestFooterAlwaysAdvertisesThePalette(t *testing.T) {
	m := newTestModel(t, sampleEntries()...)

	for _, setup := range []func(){
		func() { m.screen = screenList },
		func() { m.screen = screenDetail },
	} {
		setup()
		if !strings.Contains(m.View(), "ctrl+k") {
			t.Errorf("screen %v does not mention the palette", m.screen)
		}
	}
}
