package tui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rmpato/pogo/internal/config"
	"github.com/rmpato/pogo/internal/history"
	"github.com/rmpato/pogo/internal/store"
)

// newTestModel builds a model over a throwaway history directory, so tests
// never touch the developer's real request history.
//
// Entries are written through the real store rather than injected directly, so
// tests exercise the same load path the application uses.
func newTestModel(t *testing.T, entries ...*history.Entry) *Model {
	t.Helper()
	cfg := config.ForDir(t.TempDir())
	st, err := store.Open(cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	for _, e := range entries {
		if err := st.Append(e); err != nil {
			t.Fatalf("seed history: %v", err)
		}
	}

	cfgStore, err := config.OpenAt(filepath.Join(t.TempDir(), "config.yaml"), cfg)
	if err != nil {
		t.Fatalf("open config: %v", err)
	}
	m := New(Options{Config: cfgStore, Store: st})
	// A realistic terminal. Frames are asserted at awkward sizes too, but a
	// model built for reading content should be built at a size that fits some.
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 34})

	res, err := st.Load()
	if err != nil {
		t.Fatalf("load history: %v", err)
	}
	m.Update(entriesMsg{entries: res.Entries, skipped: res.Skipped})
	return m
}

func testEntry(method, url string, status int, opts ...func(*history.Entry)) *history.Entry {
	e := &history.Entry{
		ID:        history.NewID(),
		CreatedAt: time.Now().Add(-time.Minute),
		Source:    history.SourceRun,
		Command:   history.Command{Args: []string{"-X", method, url}},
		Request:   history.Request{Method: method, URL: url},
		Duration:  history.Duration(42 * time.Millisecond),
	}
	if status > 0 {
		e.Response = &history.Response{
			Blocks: []history.Block{{Proto: "HTTP/1.1", Status: status, Reason: "OK"}},
		}
	}
	for _, o := range opts {
		o(e)
	}
	return e
}

// The footer is the only discoverability mechanism in pogo, so its absence is a
// real bug rather than a cosmetic one.
func TestViewAlwaysShowsShortcuts(t *testing.T) {
	m := newTestModel(t, testEntry("GET", "https://api.example.com/users", 200))
	view := m.View()

	for _, want := range []string{"navigate", "inspect", "replay", "search"} {
		if !strings.Contains(view, want) {
			t.Errorf("footer is missing %q\n--- view ---\n%s", want, view)
		}
	}
}

// The rendered frame must fit the terminal exactly: one line too many scrolls
// the header away, one too few leaves a gap above the footer.
func TestViewFitsTerminalHeight(t *testing.T) {
	m := newTestModel(t, testEntry("GET", "https://api.example.com/users", 200))

	for _, size := range []tea.WindowSizeMsg{
		{Width: 100, Height: 16},
		{Width: 132, Height: 40},
		{Width: 60, Height: 12},
	} {
		m.Update(size)
		got := strings.Count(m.View(), "\n") + 1
		if got != size.Height {
			t.Errorf("at %dx%d the view is %d lines, want %d",
				size.Width, size.Height, got, size.Height)
		}
	}
}

// Rows must never exceed the pane, because a wrapped row silently doubles in
// height and pushes the rest of the list off screen.
func TestRowsNeverExceedWidth(t *testing.T) {
	long := "https://very-long-host-name.example.com/a/very/long/path/that/keeps/going/and/going?with=query&more=params"
	m := newTestModel(t,
		testEntry("GET", long, 200),
		testEntry("DELETE", "https://api.example.com/users/41", 204),
	)

	for _, width := range []int{60, 80, 100, 132, 200} {
		m.Update(tea.WindowSizeMsg{Width: width, Height: 20})
		for i, line := range strings.Split(m.View(), "\n") {
			if w := lineWidth(line); w > width {
				t.Errorf("at width %d, line %d is %d cells wide: %q", width, i, w, line)
			}
		}
	}
}

// Every screen has to fit the space between the header and the footer. A screen
// that renders taller shifts the whole frame and scrolls the header out of
// view, which looks like a crash rather than a long page.
func TestEveryScreenFitsTheFrame(t *testing.T) {
	m := newTestModel(t, testEntry("GET", "https://api.example.com/users", 200))

	screens := map[string]func(){
		"list":   func() { m.screen = screenList; m.overlay = overlayNone },
		"detail": func() { m.screen = screenDetail; m.overlay = overlayNone },
		"help":   func() { m.screen = screenHelp; m.overlay = overlayNone },
		"edit":   func() { m.screen = screenList; m.overlay = overlayNone; press(m, "e") },
		"copy":   func() { m.screen = screenList; m.overlay = overlayCopy },
		"delete": func() { m.screen = screenList; m.overlay = overlayConfirm; m.confirmID = m.selectedID() },
		"update": func() { m.screen = screenList; m.overlay = overlayUpdate; m.updateVersion = "9.9.9" },
	}

	for _, size := range []tea.WindowSizeMsg{{Width: 100, Height: 16}, {Width: 132, Height: 40}, {Width: 80, Height: 24}} {
		for name, setup := range screens {
			m.Update(size)
			setup()
			m.Update(size)

			got := strings.Count(m.View(), "\n") + 1
			if got != size.Height {
				t.Errorf("%s at %dx%d renders %d lines, want %d",
					name, size.Width, size.Height, got, size.Height)
			}
		}
	}
}
