package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func testPalette() Palette {
	return NewPalette("Commands", []PaletteCommand{
		{ID: "go:dashboard", Group: "Go to", Title: "Dashboard", Shortcut: "1"},
		{ID: "go:settings", Group: "Go to", Title: "Settings", Shortcut: "2"},
		{ID: "theme:cycle", Group: "Theme", Title: "Cycle theme", Shortcut: "t"},
	})
}

// typeKeys feeds a string to a palette one rune at a time, the way a real
// keyboard would — which is the transition worth testing, per §12.
func typeKeys(p Palette, keys string) Palette {
	for _, glyph := range keys {
		p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{glyph}})
	}
	return p
}

func TestPaletteOpensAndCloses(t *testing.T) {
	p := testPalette()
	if p.Open() {
		t.Fatal("a new palette should start closed")
	}
	p = p.Show()
	if !p.Open() {
		t.Fatal("Show should open the palette")
	}
	p, chosen := p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if p.Open() {
		t.Fatal("esc should close the palette")
	}
	if chosen != "" {
		t.Fatalf("esc must not choose anything, got %q", chosen)
	}
}

func TestPaletteEnterReturnsTheSelectedID(t *testing.T) {
	p := typeKeys(testPalette().Show(), "settings")
	p, chosen := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if chosen != "go:settings" {
		t.Fatalf("expected go:settings, got %q", chosen)
	}
	if p.Open() {
		t.Fatal("choosing should close the palette")
	}
}

func TestPaletteBackspaceWidensTheMatchSet(t *testing.T) {
	p := typeKeys(testPalette().Show(), "dash")
	narrow := len(p.matches)
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if len(p.matches) < narrow {
		t.Fatalf("backspace should match at least as much: %d then %d", narrow, len(p.matches))
	}
}

func TestPaletteCursorStaysInRange(t *testing.T) {
	p := testPalette().Show()
	// Push the cursor to the end, then filter down to fewer matches.
	for i := 0; i < 10; i++ {
		p, _ = p.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	if p.cursor >= len(p.matches) {
		t.Fatalf("cursor %d escaped %d matches", p.cursor, len(p.matches))
	}
	p = typeKeys(p, "cycle")
	if p.cursor >= len(p.matches) {
		t.Fatalf("after filtering, cursor %d escaped %d matches", p.cursor, len(p.matches))
	}
}

func TestPaletteEnterWithNoMatchesChoosesNothing(t *testing.T) {
	p := typeKeys(testPalette().Show(), "zzzzz")
	if len(p.matches) != 0 {
		t.Fatalf("expected no matches, got %d", len(p.matches))
	}
	_, chosen := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if chosen != "" {
		t.Fatalf("expected no choice, got %q", chosen)
	}
}

func TestPaletteViewFillsTheTerminalRectangle(t *testing.T) {
	p := testPalette().Show()
	for _, size := range [][2]int{{80, 24}, {110, 34}, {60, 18}} {
		block := p.View(size[0], size[1])
		lines := strings.Split(block, "\n")
		if len(lines) != size[1] {
			t.Fatalf("%dx%d: %d lines", size[0], size[1], len(lines))
		}
		for index, line := range lines {
			if got := ansi.StringWidth(line); got != size[0] {
				t.Fatalf("%dx%d: line %d is %d cells", size[0], size[1], index, got)
			}
		}
	}
}

func TestPaletteViewSurvivesAnEmptyResultSet(t *testing.T) {
	p := typeKeys(testPalette().Show(), "zzzzz")
	block := p.View(90, 28)
	if !strings.Contains(block, "no matching command") {
		t.Fatal("an empty palette should say so rather than render blank")
	}
}
