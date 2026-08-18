package tui

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// railKind distinguishes the sections of the sidebar.
type railKind int

const (
	railHeading railKind = iota
	railFilter
	railCollection
	railHost
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
func (m *Model) buildRail() {
	starred, failed := 0, 0
	collections := map[string]int{}
	hosts := map[string]int{}

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
		if h := m.displayHost(e); h != "" {
			hosts[h]++
		}
	}

	items := []railItem{
		{kind: railHeading, label: "FILTERS"},
		{kind: railFilter, label: "All", query: "", count: len(m.entries)},
		{kind: railFilter, label: "Starred", query: "is:starred", count: starred},
		{kind: railFilter, label: "Failed", query: "is:failed", count: failed},
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

	if len(hosts) > 0 {
		items = append(items, railItem{kind: railHeading, label: "HOSTS"})
		names := sortedByCount(hosts)
		// A long tail of one-off hosts would push the useful ones off screen.
		if len(names) > 8 {
			names = names[:8]
		}
		for _, name := range names {
			items = append(items, railItem{
				kind: railHost, label: name,
				query: "host:" + name, count: hosts[name],
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
			b.WriteString(styHeading.Render(truncate(item.label, width)) + "\n")
			continue
		}

		// Reserve the count's columns before laying out the label, so a long
		// host name is shortened rather than pushing the count off the edge.
		count := itoa(item.count)
		text := truncate(item.label, maxInt(3, width-3-len(count)))

		cursor := "  "
		var label string
		switch {
		case focused && i == m.railCursor:
			cursor = styCursor.Render("▌ ")
			label = stySelected.Render(text)
		case item.query != "" && m.query.Raw == item.query:
			// The row that produced what you are looking at.
			label = styOK.Render(text)
		default:
			label = styMuted.Render(text)
		}

		gap := width - lipgloss.Width(cursor) - lipgloss.Width(label) - len(count)
		if gap < 1 {
			gap = 1
		}
		b.WriteString(clampLine(cursor+label+strings.Repeat(" ", gap)+styFaint.Render(count), width) + "\n")
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
