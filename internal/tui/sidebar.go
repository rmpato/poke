package tui

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rmpato/pogo/internal/ui"
)

// railKind distinguishes the sections of the sidebar.
type railKind int

const (
	railHeading railKind = iota
	railFilter
	railCollection
	railAPI
	railEnv
)

// railItem is one line of the sidebar.
//
// The sidebar exists because collections, environments and hosts previously
// only existed while a key was held down: you had to know about `t`, `c` and
// `E` before you could discover that any of them were there. Putting the shape
// of the history on screen makes the organization something you look at rather
// than something you remember.
type railItem struct {
	kind  railKind
	label string
	query string // the search this row applies
	count int
}

func (r railItem) selectable() bool { return r.kind != railHeading }

// buildRail derives the sidebar from the history currently loaded.
//
// The APIs section is the point of it: it is the shape of what you have called,
// with each API's environments underneath it, so "which of these am I hitting?"
// is answered by looking rather than by remembering.
func (m *Model) buildRail() {
	starred, failed := 0, 0
	collections := map[string]int{}

	for _, e := range m.entries {
		if e.Favorite {
			starred++
		}
		if !e.OK() {
			failed++
		}
		if e.Collection != "" {
			collections[e.Collection]++
		}
	}

	items := []railItem{
		{kind: railHeading, label: "FILTERS"},
		{kind: railFilter, label: "All", query: "", count: len(m.entries)},
		{kind: railFilter, label: "Starred", query: "is:starred", count: starred},
		{kind: railFilter, label: "Failed", query: "is:failed", count: failed},
	}

	if summary := m.apiSummary(); len(summary) > 0 {
		items = append(items, railItem{kind: railHeading, label: "APIS"})
		// A long tail of one-off APIs would push the useful ones off screen.
		if len(summary) > 6 {
			summary = summary[:6]
		}
		for _, api := range summary {
			if api.Hidden {
				continue
			}
			items = append(items, railItem{
				kind: railAPI, label: ui.Fallback(api.Name, api.Domain),
				query: "api:" + api.Domain, count: api.Count,
			})
			// One environment is not a choice; naming it would be noise.
			if len(api.Envs) < 2 {
				continue
			}
			for _, env := range api.Envs {
				items = append(items, railItem{
					kind: railEnv, label: "  " + env.Name,
					query: "api:" + api.Domain + " env:" + env.Name, count: env.Count,
				})
			}
		}
	}

	if len(collections) > 0 {
		items = append(items, railItem{kind: railHeading, label: "COLLECTIONS"})
		for _, name := range sortedByCount(collections) {
			items = append(items, railItem{
				kind: railCollection, label: name,
				query: "collection:" + name, count: collections[name],
			})
		}
	}

	m.rail = items
	if m.railCursor >= len(items) {
		m.railCursor = len(items) - 1
	}
	if m.railCursor < 0 || (len(items) > 0 && !items[m.railCursor].selectable()) {
		m.moveRail(1)
	}
}

// sortedByCount orders names by frequency, then alphabetically so the order is
// stable between renders.
func sortedByCount(counts map[string]int) []string {
	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if counts[names[i]] != counts[names[j]] {
			return counts[names[i]] > counts[names[j]]
		}
		return names[i] < names[j]
	})
	return names
}

func (m *Model) moveRail(delta int) {
	if len(m.rail) == 0 {
		return
	}
	i := m.railCursor
	for n := 0; n < len(m.rail)*2; n++ {
		i += delta
		if i < 0 {
			i, delta = 0, 1
		}
		if i >= len(m.rail) {
			i, delta = len(m.rail)-1, -1
		}
		if m.rail[i].selectable() {
			m.railCursor = i
			return
		}
	}
}

// applyRail runs the selected sidebar row as a search, which keeps one filtering
// mechanism rather than two: the search box shows what the sidebar just did, so
// the syntax is learned by using the UI.
func (m *Model) applyRail() tea.Cmd {
	if m.railCursor >= len(m.rail) {
		return nil
	}
	item := m.rail[m.railCursor]
	if !item.selectable() {
		return nil
	}
	if item.query == "" {
		return m.doClearSearch()
	}
	return m.applyFilter(item.query)
}

// showSidebar reports whether the sidebar is both wanted and affordable.
func (m *Model) showSidebar() bool {
	return m.sidebar && m.width >= minSidebarWidth && m.screen == screenList
}

func (m *Model) sidebarWidth() int {
	if !m.showSidebar() {
		return 0
	}
	return clampInt(m.width/6, 18, 26)
}

// renderSidebar draws the rail.
func (m *Model) renderSidebar(width, height int) string {
	var b strings.Builder
	focused := m.focus == focusSidebar

	for i, item := range m.rail {
		if item.kind == railHeading {
			if i > 0 {
				b.WriteString("\n")
			}
			b.WriteString(ui.Rule(item.label, width) + "\n")
			continue
		}

		// Reserve the count's columns before laying out the label, so a long
		// API name is shortened rather than pushing the count off the edge.
		count := itoa(item.count)
		text := truncate(item.label, maxInt(3, width-3-len(count)))

		if focused && i == m.railCursor {
			b.WriteString(ui.SelectedRowStyle.Render(
				ui.StatusBar(" "+text, count+" ", width)) + "\n")
			continue
		}

		style := styMuted
		switch {
		case item.query != "" && m.query.Raw == item.query:
			style = styOK // the row that produced what you are looking at
		case item.kind == railEnv:
			style = envStyle(strings.TrimSpace(item.label))
		case item.kind == railAPI:
			style = styText
		}
		b.WriteString(ui.StatusBar(
			" "+style.Render(text), styFaint.Render(count+" "), width) + "\n")
	}

	return fitHeight(b.String(), height)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
