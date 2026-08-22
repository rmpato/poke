package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/rmpato/pogo/internal/ui"
)

// Every frame pogo draws goes through ui.PanelFrame and comes back as exactly
// the terminal's reported size (whis SYSTEM_DESIGN.md §5). Bubble Tea repaints
// the whole screen every frame, so a block that is one cell off does not
// misalign — it leaves the previous frame's pixels behind it.
//
// The chrome is the same on every screen: a status bar naming what you are
// looking at, a gradient rule, the screen, and a footer of the keys that work
// right now with the help hint on its trailing edge. Learning one screen is
// therefore learning all of them.

// View renders the whole terminal rectangle.
func (m *Model) View() string {
	if m.width < 40 || m.height < 10 {
		return ui.ClampBlock(ui.SubtitleStyle.Render("pogo needs a bigger terminal"),
			m.width, m.height)
	}

	innerW, _ := m.frameSize()

	sections := []string{
		m.header(innerW),
		ui.GradientRule(innerW),
	}
	if bar := m.filterBar(innerW); bar != "" {
		sections = append(sections, bar)
	}
	sections = append(sections,
		ui.ClampBlock(m.contentFor(m.contentHeight()), innerW, m.contentHeight()),
		m.footer(innerW),
	)

	frame := ui.PanelFrame(strings.Join(sections, "\n"), m.width, m.height)

	// Dialogs sit on top of the frame rather than replacing it: closing one
	// should not feel like arriving somewhere new.
	if box := m.overlayBox(); box != "" {
		frame = ui.OverlayOn(frame, box, m.width, m.height)
	}
	// And a notice sits on top of that, replacing the rows it covers, so what
	// the user was reading does not move as they read it.
	return m.toastOverlay(frame)
}

// frameSize is the space inside the panel border: two columns of border and
// two of padding either side, two rows of border top and bottom.
func (m *Model) frameSize() (int, int) {
	return maxInt(20, m.width-4), maxInt(6, m.height-2)
}

// contentHeight is what is left for the screen after the chrome.
func (m *Model) contentHeight() int {
	_, innerH := m.frameSize()
	h := innerH - 3 // header, rule, footer
	if m.searching || m.query.Raw != "" {
		h-- // the filter bar
	}
	return maxInt(3, h)
}

func (m *Model) contentFor(h int) string {
	innerW, _ := m.frameSize()

	switch m.screen {
	case screenHelp:
		return m.renderHelp(innerW, h)
	case screenEdit:
		return m.renderEdit(innerW, h)
	case screenDiff:
		return m.renderDiffScreen(innerW, h)
	case screenAPIs:
		return m.renderAPIs(innerW, h)
	case screenSettings:
		return m.renderSettings(innerW, h)
	case screenDetail:
		return m.renderDetail(m.selected(), innerW, h, true)
	default:
		return m.renderListScreen(h)
	}
}

// overlayBox renders the open dialog, or "" when none is.
func (m *Model) overlayBox() string {
	switch m.overlay {
	case overlayCopy:
		return m.renderCopyMenu(m.width, m.height)
	case overlayConfirm:
		return m.renderConfirm(m.width, m.height)
	case overlayUpdate:
		return m.renderUpdateConfirm(m.width, m.height)
	case overlayPalette:
		return m.renderPalette(m.width, m.height)
	case overlayEnv:
		return m.renderEnvPicker(m.width, m.height)
	case overlayCollection:
		return m.renderCollectionPrompt(m.width, m.height)
	case overlayAPIName:
		return m.renderAPINamePrompt(m.width, m.height)
	}
	return ""
}

// renderListScreen lays out the list, with the sidebar beside it when the
// terminal can afford one and a preview pane when it is wider still.
func (m *Model) renderListScreen(h int) string {
	innerW, _ := m.frameSize()
	listW := m.listWidth()

	body := m.renderList(listW, h)

	if w := m.sidebarWidth(); w > 0 {
		body = ui.JoinColumns(m.renderSidebar(w-2, h), body, w-2, 2, listW, h)
	}
	if previewW := m.previewWidth(); previewW > 0 {
		left := ui.ClampBlock(body, innerW-previewW-1, h)
		body = ui.JoinColumns(left,
			m.renderPreview(m.selected(), previewW, h),
			innerW-previewW-1, 1, previewW, h)
	}
	return body
}

func (m *Model) renderDiffScreen(width, height int) string {
	title := ui.Rule("Compare", width) + "\n" + ui.SubtitleStyle.Render(ui.FitLine(m.diffTitle, width)) + "\n"
	m.diffVP.Width = width
	m.diffVP.Height = maxInt(1, height-lipgloss.Height(title))
	return title + m.diffVP.View()
}

// header names what pogo is looking at, and where that history lives.
//
// The left half wins here, which is the opposite of the footer's rule and for
// the same reason: what you are looking at is the useful half, and the path is
// a reassurance. When both do not fit, the path goes.
func (m *Model) header(width int) string {
	left := m.headerLeft()
	right := m.headerRight()
	if right != "" && lipgloss.Width(left)+lipgloss.Width(right)+4 > width {
		right = ""
	}
	return ui.StatusBar(left, right, width)
}

// headerLeft says what pogo is looking at: the wordmark, then how many
// requests, what is filtering them, and which environment they would run in.
func (m *Model) headerLeft() string {
	left := ui.GradientBrand("POGO")

	var meta []string
	switch {
	case m.loading:
		meta = append(meta, "loading")
	case m.loadErr != nil:
		meta = append(meta, styErr.Render("history unreadable"))
	default:
		total := len(m.entries)
		if m.query.Empty() {
			meta = append(meta, pluralize(total, "request"))
		} else {
			meta = append(meta, fmt.Sprintf("%d/%d", m.visibleEntries(), total))
		}
	}
	if m.group != groupNone {
		meta = append(meta, m.group.String())
	}
	if m.envSet.Active != "" {
		meta = append(meta, ui.Tag(m.envSet.Active, ui.Primary))
	}
	if m.reveal {
		meta = append(meta, styErr.Render("secrets visible"))
	}
	if m.diffA != nil {
		meta = append(meta, ui.Tag("compare "+shortLabel(m.diffA), ui.Alt))
	}
	if m.updateVersion != "" {
		meta = append(meta, ui.Pill("update "+m.updateVersion, ui.PrimaryFg, ui.Success))
	}
	if m.skipped > 0 {
		meta = append(meta, styErr.Render(fmt.Sprintf("%d damaged records skipped", m.skipped)))
	}

	return left + "  " + ui.SubtitleStyle.Render(strings.Join(meta, " · "))
}

// headerRight names where the history being browsed actually lives. It is the
// answer to "is this the same history my shell is writing to?", which is worth
// a corner of the screen on a tool that has a POGO_HOME.
func (m *Model) headerRight() string {
	if m.st == nil {
		return ""
	}
	return ui.SubtitleStyle.Render(collapseHome(m.st.Dir()))
}

// filterBar is the `/` row: the query being typed, or the one still in force.
func (m *Model) filterBar(width int) string {
	if !m.searching && m.query.Raw == "" {
		return ""
	}
	query := m.query.Raw
	if m.searching {
		query = m.search.Value()
	}
	return ui.FilterBar(query, m.visibleEntries(), len(m.entries), width, m.searching)
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

// footer lists the shortcuts that apply right now, with the help hint on the
// trailing edge. Discoverability lives here: no menus, no modes to learn, just
// the verbs available on this screen (SYSTEM_DESIGN.md §8).
func (m *Model) footer(width int) string {
	hints := m.footerHints()
	help := ui.HelpHint()

	prefix := ""
	if m.busy != "" {
		prefix = m.spinner.View() + " " + styMuted.Render(m.busy) + "   "
	}

	// The palette hint is the answer to "what else can this do?", so it is the
	// last thing to go rather than the first — it sits at the end of the row,
	// and a naive trim from the end would drop it before anything else.
	var sticky []hint
	if n := len(hints); n > 0 && hints[n-1].key == "ctrl+k" {
		sticky, hints = hints[n-1:], hints[:n-1]
	}

	// Then drop hints from the end until the row fits, rather than truncating
	// one mid-word: half a key hint is worse than one fewer.
	for {
		var parts []string
		for _, h := range append(append([]hint{}, hints...), sticky...) {
			parts = append(parts, ui.Keycap(h.key)+" "+ui.SubtitleStyle.Render(h.desc))
		}
		line := prefix + strings.Join(parts, ui.SubtitleStyle.Render(" · "))

		if lipgloss.Width(line)+lipgloss.Width(help)+2 <= width || len(hints) == 0 {
			return ui.StatusBar(line, help, width)
		}
		hints = hints[:len(hints)-1]
	}
}

// toastOverlay draws the current notice over the finished frame.
//
// Expiry is a tea.Cmd (see clearStatus), never a goroutine, so Update stays a
// pure state transition.
func (m *Model) toastOverlay(frame string) string {
	if m.status == "" {
		return frame
	}
	kind := ui.KindSuccess
	if m.statusErr {
		kind = ui.KindDanger
	}

	toast := ui.Toast(kind, m.status, maxInt(10, m.width-8))
	rows := strings.Split(frame, "\n")
	at := len(rows) - 3 // clear of the footer and the border
	if at < 0 || at >= len(rows) {
		return frame
	}
	rows[at] = ui.FitLine("  "+toast, m.width)
	return strings.Join(rows, "\n")
}

// collapseHome shortens a path under $HOME to ~, the way a shell prompt does.
func collapseHome(path string) string {
	home, err := homeDir()
	if err != nil || home == "" || !strings.HasPrefix(path, home) {
		return path
	}
	return "~" + path[len(home):]
}
