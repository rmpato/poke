package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// View renders the whole screen: a header, the active content, and a footer of
// contextual shortcuts. The chrome is two thin rules rather than a box, which
// keeps every column available for data.
func (m *Model) View() string {
	if m.width < 40 || m.height < 10 {
		return "pogo needs a slightly larger terminal"
	}

	body := m.content()
	sections := []string{m.header(), body, m.footer()}
	return strings.Join(sections, "\n")
}

func (m *Model) content() string {
	h := m.contentHeight()

	// Overlays replace the content area entirely; they are modal, and dimming
	// the background is not something a terminal does convincingly.
	switch m.overlay {
	case overlayCopy:
		return m.renderCopyMenu(m.width, h)
	case overlayConfirm:
		return m.renderConfirm(m.width, h)
	}

	switch m.screen {
	case screenHelp:
		return m.renderHelp(m.width, h)
	case screenEdit:
		return m.renderEdit(m.width, h)
	case screenDiff:
		return m.renderDiffScreen(m.width, h)
	case screenDetail:
		return m.renderDetail(m.selected(), m.width, h, true)
	default:
		return m.renderListScreen(h)
	}
}

// renderListScreen lays out the history list, adding a preview pane when the
// terminal is wide enough to carry one without squeezing the list.
func (m *Model) renderListScreen(h int) string {
	previewW := m.previewWidth()
	if previewW == 0 {
		return m.renderList(m.width, h)
	}

	listW := m.listWidth()
	left := m.renderList(listW, h)
	right := m.renderDetail(m.selected(), previewW-2, h, false)

	divider := strings.TrimRight(strings.Repeat(styRule.Render("│")+"\n", h), "\n")
	return lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(listW).Render(left),
		divider,
		lipgloss.NewStyle().Width(previewW-1).PaddingLeft(1).Render(right),
	)
}

func (m *Model) renderDiffScreen(width, height int) string {
	title := styHeading.Render("COMPARE") + "  " + styMuted.Render(m.diffTitle) + "\n"
	m.diffVP.Width = width
	m.diffVP.Height = maxInt(1, height-lipgloss.Height(title))
	return title + m.diffVP.View()
}

// header shows what pogo is looking at: how many requests, what is filtering
// them, and where they came from.
func (m *Model) header() string {
	left := styTitle.Render("POGO")

	var meta []string
	switch {
	case m.loading:
		meta = append(meta, styFaint.Render("loading"))
	case m.loadErr != nil:
		meta = append(meta, styErr.Render("history unreadable"))
	default:
		total := len(m.entries)
		if m.query.Empty() {
			meta = append(meta, styMuted.Render(pluralize(total, "request")))
		} else {
			meta = append(meta, styMuted.Render(fmt.Sprintf("%d/%d", m.visibleEntries(), total)))
		}
	}
	if m.grouped {
		meta = append(meta, styFaint.Render("grouped"))
	}
	if m.reveal {
		meta = append(meta, styErr.Render("secrets visible"))
	}
	if m.diffA != nil {
		meta = append(meta, styBadge.Render("compare: "+shortLabel(m.diffA)))
	}
	if m.skipped > 0 {
		meta = append(meta, styErr.Render(fmt.Sprintf("%d damaged records skipped", m.skipped)))
	}

	right := styFaint.Render(collapseHome(m.st.Dir()))
	center := strings.Join(meta, styFaint.Render(" · "))

	line := left + "  " + center
	if gap := m.width - lipgloss.Width(line) - lipgloss.Width(right); gap > 2 {
		line += strings.Repeat(" ", gap) + right
	}

	out := clampLine(line, m.width) + "\n" + rule(m.width)
	if m.searching || m.status != "" {
		out += "\n" + m.statusBar()
	}
	return out
}

// statusBar carries either the search input or a transient message. They share
// a line because they are never both interesting at once.
func (m *Model) statusBar() string {
	if m.searching {
		return m.search.View()
	}
	style := styOK
	if m.statusErr {
		style = styErr
	}
	return clampLine(style.Render(m.status), m.width)
}

func (m *Model) visibleEntries() int {
	n := 0
	for _, r := range m.rows {
		if r.entry != nil {
			n++
		}
	}
	return n
}

// footer lists the shortcuts that apply right now. Discoverability lives here:
// no menus, no modes to learn, just the verbs available on this screen.
func (m *Model) footer() string {
	var parts []string
	for _, h := range m.footerHints() {
		parts = append(parts, styKey.Render(h.key)+" "+styKeyHint.Render(h.desc))
	}
	line := strings.Join(parts, styFaint.Render("   "))

	if m.busy != "" {
		line = m.spinner.View() + " " + styMuted.Render(m.busy) + styFaint.Render("   ") + line
	}
	return rule(m.width) + "\n" + clampLine(line, m.width)
}

// collapseHome shortens a path under $HOME to ~, the way a shell prompt does.
func collapseHome(path string) string {
	home, err := homeDir()
	if err != nil || home == "" || !strings.HasPrefix(path, home) {
		return path
	}
	return "~" + path[len(home):]
}
