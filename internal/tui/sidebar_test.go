package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rmpato/poke/internal/history"
)

func railEntries() []*history.Entry {
	mk := func(host, path string, status int, fav bool, collection string) *history.Entry {
		e := &history.Entry{
			ID:        history.NewID(),
			CreatedAt: time.Now(),
			Request:   history.Request{Method: "GET", URL: "https://" + host + path},
			Favorite:  fav, Collection: collection,
		}
		if status > 0 {
			e.Response = &history.Response{Blocks: []history.Block{{Status: status}}}
		} else {
			e.Exit = 7
		}
		return e
	}
	return []*history.Entry{
		mk("api.foo.com", "/users", 200, true, "users"),
		mk("api.foo.com", "/users/42", 200, false, "users"),
		mk("api.foo.com", "/login", 401, false, "auth"),
		mk("api.bar.com", "/things", 500, false, ""),
		mk("api.bar.com", "/dead", 0, false, ""),
	}
}

// The sidebar exists so the shape of the history is visible without knowing
// which key reveals it. That only works if the counts are right.
func TestSidebarSummarizesHistory(t *testing.T) {
	m := newTestModel(t, railEntries()...)
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 24})

	find := func(label string) (railItem, bool) {
		for _, it := range m.rail {
			if it.label == label {
				return it, true
			}
		}
		return railItem{}, false
	}

	for label, want := range map[string]int{
		"All": 5, "Starred": 1, "Failed": 3, // 401, 500 and the curl failure
		"users": 2, "auth": 1,
		"api.foo.com": 3, "api.bar.com": 2,
	} {
		item, ok := find(label)
		if !ok {
			t.Errorf("sidebar has no row for %q", label)
			continue
		}
		if item.count != want {
			t.Errorf("%s = %d, want %d", label, item.count, want)
		}
	}

	view := m.View()
	for _, want := range []string{"FILTERS", "COLLECTIONS", "HOSTS"} {
		if !strings.Contains(view, want) {
			t.Errorf("sidebar is missing the %q section", want)
		}
	}
}

// Selecting a row runs it as a search, so the search box shows the syntax that
// produced the result — which is how the filter language gets learned.
func TestSidebarSelectionAppliesAndTeachesTheQuery(t *testing.T) {
	m := newTestModel(t, railEntries()...)
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 24})

	press(m, "tab")
	if m.focus != focusSidebar {
		t.Fatal("tab should move focus to the sidebar")
	}

	// Walk to the starred filter and apply it.
	for i := 0; i < 20; i++ {
		if m.rail[m.railCursor].label == "Starred" {
			break
		}
		m.moveRail(1)
	}
	press(m, "enter")

	if m.query.Raw != "is:starred" {
		t.Errorf("query = %q, want is:starred", m.query.Raw)
	}
	if m.search.Value() != "is:starred" {
		t.Error("the search box should show the query the sidebar just ran")
	}
	if m.visibleEntries() != 1 {
		t.Errorf("%d entries visible, want the single starred one", m.visibleEntries())
	}
}

func TestSidebarHidesItselfOnNarrowTerminals(t *testing.T) {
	m := newTestModel(t, railEntries()...)

	m.Update(tea.WindowSizeMsg{Width: 140, Height: 24})
	if !m.showSidebar() {
		t.Error("a wide terminal should show the sidebar")
	}
	withSidebar := m.listWidth()

	m.Update(tea.WindowSizeMsg{Width: 90, Height: 24})
	if m.showSidebar() {
		t.Error("a narrow terminal should not show the sidebar")
	}

	// At the same width, dropping the sidebar must hand its columns to the list.
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 24})
	press(m, "\\")
	if m.listWidth() <= withSidebar {
		t.Errorf("list is %d columns without the sidebar and %d with it; it should be wider",
			m.listWidth(), withSidebar)
	}
	press(m, "\\")
	// And the frame still fits exactly.
	if got := strings.Count(m.View(), "\n") + 1; got != 24 {
		t.Errorf("view is %d lines, want 24", got)
	}
}

func TestSidebarToggle(t *testing.T) {
	m := newTestModel(t, railEntries()...)
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 24})

	press(m, "\\")
	if m.showSidebar() {
		t.Error("\\ should hide the sidebar")
	}
	if m.focus != focusList {
		t.Error("hiding the sidebar must return focus to the list")
	}
	press(m, "\\")
	if !m.showSidebar() {
		t.Error("\\ should bring it back")
	}
}

// Replaying arms the comparison against what was replayed, so the obvious
// follow-up question — what changed? — is one keypress rather than four.
func TestReplayArmsTheComparison(t *testing.T) {
	m := newTestModel(t, railEntries()...)
	source := m.selected()

	press(m, "r")
	if m.replaySource == nil || m.replaySource.ID != source.ID {
		t.Fatal("replay should remember what it came from")
	}

	m.Update(replayMsg{id: "NEW", summary: "→ 200 OK · 18ms"})

	if m.diffA == nil || m.diffA.ID != source.ID {
		t.Error("after a replay, the original should be marked for comparison")
	}
	if !strings.Contains(m.View(), "compare with the original") {
		t.Error("the user should be told that d now compares the two")
	}
}

func TestReplayDoesNotStealAnExistingComparisonMark(t *testing.T) {
	m := newTestModel(t, railEntries()...)
	marked := m.entries[2]
	m.diffA = marked

	press(m, "r")
	m.Update(replayMsg{id: "NEW", summary: "→ 200 OK"})

	if m.diffA.ID != marked.ID {
		t.Error("a comparison the user set up by hand must win")
	}
}
