package tui

import (
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/rmpato/poke/internal/history"
)

// rebuildRows applies the current query and grouping to the loaded history.
func (m *Model) rebuildRows() {
	m.rows = m.rows[:0]

	filtered := make([]int, 0, len(m.entries))
	for i, e := range m.entries {
		if m.query.Empty() || m.query.Match(e) {
			filtered = append(filtered, i)
		}
	}

	if m.group == groupNone {
		for _, i := range filtered {
			m.rows = append(m.rows, row{entry: m.entries[i]})
		}
		m.clampCursor()
		return
	}

	// Groups are ordered by their most recent request. History is already
	// newest-first, so first appearance is the right order and nothing needs
	// sorting by time.
	order := make([]string, 0, 8)
	members := map[string][]int{}
	for _, i := range filtered {
		host := m.groupKey(m.entries[i])
		if _, seen := members[host]; !seen {
			order = append(order, host)
		}
		members[host] = append(members[host], i)
	}

	// The unfiled bucket sorts last: it is a leftover, not a collection, and
	// putting it between named groups makes the list read like a mistake.
	if m.group == groupCollection {
		for i, name := range order {
			if name == noCollection && i != len(order)-1 {
				order = append(append(order[:i], order[i+1:]...), noCollection)
				break
			}
		}
	}

	for _, host := range order {
		idx := members[host]
		m.rows = append(m.rows, row{header: true, group: host, count: len(idx)})
		if m.collapsed[host] {
			continue
		}
		for _, i := range idx {
			m.rows = append(m.rows, row{entry: m.entries[i], group: host})
		}
	}
	m.clampCursor()
}

// groupKey is the heading a request belongs under.
// noCollection labels requests that have not been filed anywhere.
const noCollection = "(no collection)"

func (m *Model) groupKey(e *history.Entry) string {
	if m.group == groupCollection {
		if e.Collection == "" {
			return noCollection
		}
		return e.Collection
	}
	if host := m.displayHost(e); host != "" {
		return host
	}
	return "(unknown host)"
}

func (m *Model) clampCursor() {
	if m.cursor >= len(m.rows) {
		m.cursor = maxInt(0, len(m.rows)-1)
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.ensureVisible()
}

func (m *Model) ensureVisible() {
	h := m.contentHeight()
	if m.cursor < m.top {
		m.top = m.cursor
	}
	if m.cursor >= m.top+h {
		m.top = m.cursor - h + 1
	}
	if m.top < 0 {
		m.top = 0
	}
	if max := len(m.rows) - h; m.top > max {
		m.top = maxInt(0, max)
	}
}

func (m *Model) move(delta int) {
	if len(m.rows) == 0 {
		return
	}
	m.cursor = clampInt(m.cursor+delta, 0, len(m.rows)-1)
	m.ensureVisible()
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// listColumns holds the width of each column in a history row.
//
// Every fixed column is accounted for exactly, including the single spaces
// between them, so the assembled row is never wider than the pane. A row that
// wraps would silently double the height of the list and push the header off
// screen.
type listColumns struct {
	method, host, path, status, dur, size, age int
	hostGap                                    int
}

// rowOverhead is the chrome around the flexible columns: a two-cell cursor, the
// star cell, the space after it, and the four separators before status,
// duration, size and age.
const rowOverhead = 2 + 1 + 1 + 4

func computeColumns(width int, grouped bool) listColumns {
	c := listColumns{method: 7, status: 4, dur: 7, size: 7, age: 5}

	avail := width - rowOverhead - c.method - c.status - c.dur - c.size - c.age
	if avail < 8 {
		// Very narrow pane: drop the columns that are nice rather than
		// necessary, so the path always survives.
		c.size, c.age = 0, 0
		avail = width - rowOverhead - c.method - c.status - c.dur + 2
	}

	if grouped {
		// The host is already in the group header above; give its width to the
		// path, which is what distinguishes rows within a group.
		c.path = maxInt(6, avail)
		return c
	}

	c.host = clampInt(avail/3, 8, 26)
	c.hostGap = 1
	c.path = maxInt(6, avail-c.host-c.hostGap)
	return c
}

// renderList draws the history list.
func (m *Model) renderList(width, height int) string {
	if m.loading {
		return m.centerNotice(width, height, m.spinner.View()+" reading history…")
	}
	if m.loadErr != nil {
		return m.centerNotice(width, height, styErr.Render("could not read history: ")+m.loadErr.Error())
	}
	if len(m.entries) == 0 {
		return m.emptyHistory(width, height)
	}
	if len(m.rows) == 0 {
		return m.centerNotice(width, height,
			styMuted.Render("no requests match ")+styText.Render(m.query.Raw)+
				"\n\n"+styMuted.Render("press esc to clear the search"))
	}

	cols := computeColumns(width, m.group == groupHost)
	lines := make([]string, 0, height)

	end := minInt(m.top+height, len(m.rows))
	for i := m.top; i < end; i++ {
		lines = append(lines, m.renderRow(m.rows[i], i == m.cursor, cols, width))
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (m *Model) renderRow(r row, selected bool, cols listColumns, width int) string {
	if r.header {
		return m.renderGroupHeader(r, selected, width)
	}
	e := r.entry

	cursor := "  "
	if selected {
		cursor = styCursor.Render("▌ ")
	}

	mark := " "
	if e.Favorite {
		mark = styStar.Render("★")
	}

	var b strings.Builder
	b.WriteString(cursor)
	b.WriteString(mark)
	b.WriteString(" ")
	b.WriteString(methodStyle(e.Request.Method).Render(pad(e.Request.Method, cols.method)))

	if cols.host > 0 {
		b.WriteString(styMuted.Render(pad(truncate(m.displayHost(e), cols.host), cols.host)))
		b.WriteString(strings.Repeat(" ", cols.hostGap))
	}

	b.WriteString(styText.Render(pad(truncateMiddle(m.displayPath(e), cols.path), cols.path)))
	b.WriteString(" ")
	b.WriteString(statusStyle(e.Status()).Render(padLeft(statusText(e.Status(), e.Exit), cols.status)))
	b.WriteString(" ")
	b.WriteString(styMuted.Render(padLeft(e.Duration.String(), cols.dur)))

	if cols.size > 0 {
		size := "—"
		if e.Response != nil && e.Response.Body != nil {
			size = bytesHuman(e.Response.Body.Size)
		}
		b.WriteString(" ")
		b.WriteString(styFaint.Render(padLeft(size, cols.size)))
	}
	if cols.age > 0 {
		b.WriteString(" ")
		b.WriteString(styFaint.Render(padLeft(age(e.CreatedAt, m.now), cols.age)))
	}

	return clampLine(b.String(), width)
}

func (m *Model) renderGroupHeader(r row, selected bool, width int) string {
	arrow := "▾"
	if m.collapsed[r.group] {
		arrow = "▸"
	}
	cursor := "  "
	if selected {
		cursor = styCursor.Render("▌ ")
	}
	label := styHeading.Render(truncate(r.group, maxInt(10, width-16)))
	count := styFaint.Render(pluralize(r.count, "request"))
	gap := width - lipgloss.Width(cursor+arrow+" "+label+count) - 2
	if gap < 1 {
		gap = 1
	}
	return clampLine(cursor+styFaint.Render(arrow)+" "+label+strings.Repeat(" ", gap)+count, width)
}

// displayURL applies the redaction policy before anything reaches the screen,
// so a token embedded in a query string is masked in the list as well as in the
// detail view. Pressing S reveals the originals.
func (m *Model) displayURL(e *history.Entry) string {
	if m.reveal {
		return e.Request.URL
	}
	return m.cfg.Redact.MaskURL(e.Request.URL)
}

func (m *Model) displayHost(e *history.Entry) string { return history.HostOf(m.displayURL(e)) }
func (m *Model) displayPath(e *history.Entry) string { return history.PathOf(m.displayURL(e)) }

func pluralize(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return strconv.Itoa(n) + " " + word + "s"
}

// emptyHistory is what a new user sees. An empty state is a teaching moment,
// so it shows the one command that fills the screen with something.
func (m *Model) emptyHistory(width, height int) string {
	lines := []string{
		styHeading.Render("No requests yet"),
		"",
		styMuted.Render("pogo shows what ") + styKey.Render("poke") + styMuted.Render(" has run. Try:"),
		"",
		"    " + styKey.Render("poke https://api.github.com/zen"),
		"",
		styMuted.Render("poke passes everything through to curl, so any curl"),
		styMuted.Render("command works and every one of them lands here."),
		"",
		styFaint.Render("history: " + m.st.Path()),
	}
	return m.centerNotice(width, height, strings.Join(lines, "\n"))
}

func (m *Model) centerNotice(width, height int, content string) string {
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}

// knownCollections lists the collections already in use, so the prompt can
// suggest them instead of making the user remember exact spelling.
func (m *Model) knownCollections() []string {
	seen := map[string]bool{}
	var out []string
	for _, e := range m.entries {
		if e.Collection != "" && !seen[e.Collection] {
			seen[e.Collection] = true
			out = append(out, e.Collection)
		}
	}
	sort.Strings(out)
	if len(out) > 6 {
		out = append(out[:6], "…")
	}
	return out
}
