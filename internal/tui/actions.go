package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// The verbs. Every one of these is reachable three ways — its key, the command
// palette, and the help reference — and all three call the method here, so they
// cannot drift apart.

func (m *Model) doInspect() tea.Cmd {
	// On a group header the natural meaning of "open" is fold, not inspect.
	if m.cursor < len(m.rows) && m.rows[m.cursor].header {
		g := m.rows[m.cursor].group
		m.collapsed[g] = !m.collapsed[g]
		m.rebuildRows()
		return nil
	}
	if m.selected() == nil {
		return nil
	}
	m.screen = screenDetail
	m.layout()
	return m.loadDetail()
}

func (m *Model) doReplay() tea.Cmd {
	e := m.selected()
	if e == nil {
		return nil
	}
	m.busy = "replaying"
	m.replaySource = e
	return tea.Batch(replay(m.recorder(), e), m.spinner.Tick)
}

func (m *Model) doEdit() tea.Cmd {
	return m.startEdit(m.selected())
}

func (m *Model) doCopy() tea.Cmd {
	if m.selected() == nil {
		return nil
	}
	m.overlay = overlayCopy
	m.copyCursor = 0
	return nil
}

func (m *Model) doDiff() tea.Cmd {
	e := m.selected()
	if e == nil {
		return nil
	}
	return m.toggleDiff(e)
}

func (m *Model) doDelete() tea.Cmd {
	e := m.selected()
	if e == nil {
		return nil
	}
	m.overlay = overlayConfirm
	m.confirmID = e.ID
	return nil
}

func (m *Model) doStar() tea.Cmd {
	e := m.selected()
	if e == nil {
		return nil
	}
	return setFavorite(m.st, e.ID, !e.Favorite)
}

func (m *Model) doCollection() tea.Cmd {
	e := m.selected()
	if e == nil {
		return nil
	}
	m.overlay = overlayCollection
	m.collectionInput.SetValue(e.Collection)
	m.collectionInput.CursorEnd()
	return m.collectionInput.Focus()
}

func (m *Model) doGroup() tea.Cmd {
	m.group = (m.group + 1) % 3
	id := m.selectedID()
	m.rebuildRows()
	m.selectID(id)
	m.flash(m.group.String())
	return clearStatus(m.statusTok)
}

func (m *Model) doSearch() tea.Cmd {
	m.screen = screenList
	m.searching = true
	m.search.SetValue(m.query.Raw)
	m.search.CursorEnd()
	return m.search.Focus()
}

func (m *Model) doClearSearch() tea.Cmd {
	if m.query.Raw == "" {
		return nil
	}
	m.query = Query{}
	m.search.SetValue("")
	m.rebuildRows()
	m.flash("search cleared")
	return clearStatus(m.statusTok)
}

// applyFilter runs a canned search, which is how the palette offers things like
// "show failures only" without the user learning the filter syntax first. The
// syntax then shows up in the search bar, which is how they learn it.
func (m *Model) applyFilter(expr string) tea.Cmd {
	m.screen = screenList
	m.query = ParseQuery(expr)
	m.search.SetValue(expr)
	id := m.selectedID()
	m.rebuildRows()
	m.selectID(id)
	m.flash("filter: " + expr)
	return clearStatus(m.statusTok)
}

func (m *Model) doToggleSidebar() tea.Cmd {
	m.sidebar = !m.sidebar
	if !m.sidebar {
		m.focus = focusList
	}
	m.layout()
	if m.sidebar && m.width < minSidebarWidth {
		m.flash("the terminal is too narrow for the sidebar")
		return clearStatus(m.statusTok)
	}
	return nil
}

func (m *Model) doReveal() tea.Cmd {
	m.reveal = !m.reveal
	if m.reveal {
		m.flashErr("secrets revealed — press S to hide")
	} else {
		m.flash("secrets hidden")
	}
	return clearStatus(m.statusTok)
}

func (m *Model) doBodyMode() tea.Cmd {
	m.detail.mode = (m.detail.mode + 1) % 3
	m.flash("body view: " + bodyModeNames[m.detail.mode])
	return clearStatus(m.statusTok)
}

func (m *Model) doEnv() tea.Cmd {
	if len(m.envSet.Names()) == 0 {
		m.flashErr("no environments defined — see docs/environments.md")
		return clearStatus(m.statusTok)
	}
	m.overlay = overlayEnv
	m.envCursor = 0
	for i, name := range m.envSet.Names() {
		if name == m.envSet.Active {
			m.envCursor = i + 1 // the first row is "(none)"
		}
	}
	return nil
}

func (m *Model) doUpdate() tea.Cmd {
	if m.updateVersion == "" {
		m.flash("no update available")
		return clearStatus(m.statusTok)
	}
	m.overlay = overlayUpdate
	return nil
}

func (m *Model) doHelp() tea.Cmd {
	if m.screen != screenHelp {
		m.prevScreen = m.screen
	}
	m.screen = screenHelp
	return nil
}

// doPalette opens the command palette: the answer to "what can this thing do?"
// without having to already know.
func (m *Model) doPalette() tea.Cmd {
	m.overlay = overlayPalette
	m.paletteCursor = 0
	m.paletteInput.SetValue("")
	return m.paletteInput.Focus()
}
