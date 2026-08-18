package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/rmpato/poke/internal/curlargs"
	"github.com/rmpato/poke/internal/version"
)

// handleKey routes a keypress to the active overlay or screen. Overlays are
// modal and get first refusal, which is what makes "esc always goes back" true.
func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.overlay != overlayNone {
		return m.handleOverlayKey(msg)
	}
	if m.searching {
		return m.handleSearchKey(msg)
	}

	switch m.screen {
	case screenEdit:
		return m.handleEditKey(msg)
	case screenDetail:
		return m.handleDetailKey(msg)
	case screenDiff:
		return m.handleDiffKey(msg)
	case screenHelp:
		switch {
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit
		default:
			m.screen = m.prevScreen
			return m, nil
		}
	default:
		return m.handleListKey(msg)
	}
}

// --- list ------------------------------------------------------------------

func (m *Model) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Quit):
		return m, tea.Quit

	case key.Matches(msg, keys.Help):
		m.prevScreen, m.screen = m.screen, screenHelp
		return m, nil

	case key.Matches(msg, keys.Up):
		m.move(-1)
		return m, m.previewCmd()
	case key.Matches(msg, keys.Down):
		m.move(1)
		return m, m.previewCmd()
	case key.Matches(msg, keys.PageUp):
		m.move(-m.contentHeight() / 2)
		return m, m.previewCmd()
	case key.Matches(msg, keys.PageDn):
		m.move(m.contentHeight() / 2)
		return m, m.previewCmd()
	case key.Matches(msg, keys.Top):
		m.cursor = 0
		m.ensureVisible()
		return m, m.previewCmd()
	case key.Matches(msg, keys.Bottom):
		m.cursor = maxInt(0, len(m.rows)-1)
		m.ensureVisible()
		return m, m.previewCmd()

	case key.Matches(msg, keys.Enter):
		// On a group header, Enter folds; on a request, it inspects.
		if m.cursor < len(m.rows) && m.rows[m.cursor].header {
			g := m.rows[m.cursor].group
			m.collapsed[g] = !m.collapsed[g]
			m.rebuildRows()
			return m, nil
		}
		if m.selected() == nil {
			return m, nil
		}
		m.screen = screenDetail
		m.layout()
		return m, m.loadDetail()

	case key.Matches(msg, keys.Search):
		m.searching = true
		m.search.SetValue(m.query.Raw)
		m.search.CursorEnd()
		return m, m.search.Focus()

	case key.Matches(msg, keys.Back):
		if m.query.Raw != "" {
			m.query = Query{}
			m.search.SetValue("")
			m.rebuildRows()
			m.flash("search cleared")
			return m, clearStatus(m.statusTok)
		}
		return m, nil

	case msg.String() == "u":
		// Updating replaces the binaries on disk, so it always asks first.
		if m.updateVersion == "" {
			m.flash("no update available")
			return m, clearStatus(m.statusTok)
		}
		m.overlay = overlayUpdate
		return m, nil

	case key.Matches(msg, keys.Group):
		m.grouped = !m.grouped
		id := m.selectedID()
		m.rebuildRows()
		m.selectID(id)
		if m.grouped {
			m.flash("grouped by host")
		} else {
			m.flash("chronological")
		}
		return m, clearStatus(m.statusTok)

	default:
		return m.handleEntryAction(msg)
	}
}

// previewCmd loads payloads for the side preview, but only when there is one.
func (m *Model) previewCmd() tea.Cmd {
	if m.previewWidth() == 0 {
		return nil
	}
	return m.loadDetail()
}

// handleEntryAction covers the verbs that work on the selected request from
// both the list and the detail view.
func (m *Model) handleEntryAction(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	e := m.selected()
	if e == nil {
		return m, nil
	}

	switch {
	case key.Matches(msg, keys.Replay):
		m.busy = "replaying"
		return m, tea.Batch(replay(m.rec, e), m.spinner.Tick)

	case key.Matches(msg, keys.Edit):
		m.startEdit(e)
		return m, nil

	case key.Matches(msg, keys.Star):
		return m, setFavorite(m.st, e.ID, !e.Favorite)

	case key.Matches(msg, keys.Delete):
		m.overlay = overlayConfirm
		m.confirmID = e.ID
		return m, nil

	case key.Matches(msg, keys.Copy):
		m.overlay = overlayCopy
		m.copyCursor = 0
		return m, nil

	case key.Matches(msg, keys.Diff):
		return m, m.toggleDiff(e)

	case key.Matches(msg, keys.Reveal):
		m.reveal = !m.reveal
		if m.reveal {
			m.flashErr("secrets revealed — press S to hide")
		} else {
			m.flash("secrets hidden")
		}
		return m, clearStatus(m.statusTok)
	}
	return m, nil
}

// --- search ----------------------------------------------------------------

func (m *Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.searching = false
		m.search.Blur()
		return m, nil
	case "enter":
		m.searching = false
		m.search.Blur()
		return m, nil
	}

	var cmd tea.Cmd
	m.search, cmd = m.search.Update(msg)

	// Filtering happens on every keystroke: search that waits for Enter feels
	// broken when the whole point is to find something in a hundred rows.
	m.query = ParseQuery(m.search.Value())
	id := m.selectedID()
	m.rebuildRows()
	m.selectID(id)
	return m, cmd
}

// --- detail ----------------------------------------------------------------

func (m *Model) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Quit):
		return m, tea.Quit

	case key.Matches(msg, keys.Back):
		m.screen = screenList
		m.layout()
		return m, nil

	case key.Matches(msg, keys.Help):
		m.prevScreen, m.screen = m.screen, screenHelp
		return m, nil

	case key.Matches(msg, keys.NextTab):
		m.detail.tab = (m.detail.tab + 1) % detailTab(len(tabNames))
		m.detail.vp.GotoTop()
		return m, nil
	case key.Matches(msg, keys.PrevTab):
		m.detail.tab = (m.detail.tab + detailTab(len(tabNames)) - 1) % detailTab(len(tabNames))
		m.detail.vp.GotoTop()
		return m, nil

	case msg.String() >= "1" && msg.String() <= "5" && len(msg.String()) == 1:
		m.detail.tab = detailTab(msg.String()[0] - '1')
		m.detail.vp.GotoTop()
		return m, nil

	case key.Matches(msg, keys.Body):
		m.detail.mode = (m.detail.mode + 1) % 3
		m.flash("body view: " + bodyModeNames[m.detail.mode])
		return m, clearStatus(m.statusTok)

	case key.Matches(msg, keys.Toggle):
		if m.detail.mode == bodyTree {
			m.toggleTreeNode()
			return m, nil
		}

	case key.Matches(msg, keys.Down):
		if m.treeNavigable() {
			m.moveTreeCursor(1)
			return m, nil
		}
		m.detail.vp.ScrollDown(1)
		return m, nil

	case key.Matches(msg, keys.Up):
		if m.treeNavigable() {
			m.moveTreeCursor(-1)
			return m, nil
		}
		m.detail.vp.ScrollUp(1)
		return m, nil

	case key.Matches(msg, keys.PageDn):
		m.detail.vp.HalfPageDown()
		return m, nil
	case key.Matches(msg, keys.PageUp):
		m.detail.vp.HalfPageUp()
		return m, nil
	case key.Matches(msg, keys.Top):
		m.detail.vp.GotoTop()
		return m, nil
	case key.Matches(msg, keys.Bottom):
		m.detail.vp.GotoBottom()
		return m, nil
	}

	return m.handleEntryAction(msg)
}

// treeNavigable reports whether j/k should move a JSON cursor rather than
// scroll the pane.
func (m *Model) treeNavigable() bool {
	return m.detail.mode == bodyTree &&
		m.detail.tab != tabTiming && m.detail.tab != tabRaw &&
		len(m.detail.focused().lines) > 0
}

func (m *Model) moveTreeCursor(delta int) {
	ts := m.detail.focused()
	if len(ts.lines) == 0 {
		return
	}
	ts.cursor = clampInt(ts.cursor+delta, 0, len(ts.lines)-1)

	// Keep the cursor inside the viewport as it moves.
	if ts.cursor < m.detail.vp.YOffset {
		m.detail.vp.SetYOffset(ts.cursor)
	}
	if bottom := m.detail.vp.YOffset + m.detail.vp.Height - 1; ts.cursor > bottom {
		m.detail.vp.SetYOffset(ts.cursor - m.detail.vp.Height + 1)
	}
}

// toggleTreeNode folds or unfolds the node under the cursor. Pressing space on
// a leaf folds its parent, which is what "collapse this bit" usually means.
func (m *Model) toggleTreeNode() {
	ts := m.detail.focused()
	if ts.cursor >= len(ts.lines) {
		return
	}
	line := ts.lines[ts.cursor]
	if line.node.container() {
		line.node.expanded = !line.node.expanded
		return
	}
	if parent := findParent(ts.tree, line.node); parent != nil {
		parent.expanded = false
		for i, l := range ts.lines {
			if l.node == parent {
				ts.cursor = i
				break
			}
		}
	}
}

func findParent(root, target *jnode) *jnode {
	for _, c := range root.children {
		if c == target {
			return root
		}
		if p := findParent(c, target); p != nil {
			return p
		}
	}
	return nil
}

// --- edit ------------------------------------------------------------------

func (m *Model) handleEditKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Run):
		return m, m.runEditor()

	case key.Matches(msg, keys.Editor):
		return m, openEditor(m.editor.Value())

	case msg.Type == tea.KeyEsc:
		m.screen = screenList
		m.editor.Blur()
		m.layout()
		return m, nil
	}

	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(msg)
	return m, cmd
}

// runEditor parses the edited command and executes it as a new entry. The
// original is never touched.
func (m *Model) runEditor() tea.Cmd {
	text := strings.TrimSpace(m.editor.Value())
	if text == "" {
		m.flashErr("nothing to run")
		return clearStatus(m.statusTok)
	}

	args, err := curlargs.Split(text)
	if err != nil {
		m.flashErr(err.Error())
		return clearStatus(m.statusTok)
	}
	args = curlargs.StripCurl(args)
	if len(args) == 0 {
		m.flashErr("no curl arguments found")
		return clearStatus(m.statusTok)
	}

	parent := m.entryByID(m.editID)
	m.screen = screenList
	m.editor.Blur()
	m.busy = "running"
	m.layout()
	return tea.Batch(runEdited(m.rec, parent, args), m.spinner.Tick)
}

// --- diff ------------------------------------------------------------------

func (m *Model) handleDiffKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, keys.Back):
		m.screen = screenList
		m.layout()
		return m, nil
	case key.Matches(msg, keys.Diff):
		m.diffA = nil
		m.screen = screenList
		m.flash("comparison cleared")
		m.layout()
		return m, clearStatus(m.statusTok)
	}
	var cmd tea.Cmd
	m.diffVP, cmd = m.diffVP.Update(msg)
	return m, cmd
}

// --- overlays --------------------------------------------------------------

func (m *Model) handleOverlayKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.overlay {
	case overlayConfirm:
		switch msg.String() {
		case "y", "Y", "enter":
			id := m.confirmID
			m.overlay = overlayNone
			m.confirmID = ""
			if m.screen == screenDetail {
				m.screen = screenList
			}
			return m, deleteEntry(m.st, id)
		default:
			m.overlay = overlayNone
			m.confirmID = ""
			return m, nil
		}

	case overlayUpdate:
		switch msg.String() {
		case "y", "Y", "enter":
			m.overlay = overlayNone
			m.updating = true
			m.busy = "updating"
			return m, tea.Batch(applyUpdate(version.Version), m.spinner.Tick)
		default:
			m.overlay = overlayNone
			return m, nil
		}

	case overlayCopy:
		items := m.copyItems()
		switch {
		case key.Matches(msg, keys.Down):
			m.copyCursor = clampInt(m.copyCursor+1, 0, len(items)-1)
			return m, nil
		case key.Matches(msg, keys.Up):
			m.copyCursor = clampInt(m.copyCursor-1, 0, len(items)-1)
			return m, nil
		case key.Matches(msg, keys.Enter):
			m.overlay = overlayNone
			return m, m.runCopy(items[m.copyCursor])
		case msg.String() == "esc" || key.Matches(msg, keys.Copy):
			m.overlay = overlayNone
			return m, nil
		default:
			// Direct selection by the item's own letter.
			for _, it := range items {
				if msg.String() == it.key {
					m.overlay = overlayNone
					return m, m.runCopy(it)
				}
			}
			return m, nil
		}
	}
	m.overlay = overlayNone
	return m, nil
}
