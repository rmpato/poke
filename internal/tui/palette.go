package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/rmpato/poke/internal/ui"
)

// The palette is the kit's (internal/ui), driven by pogo's command registry.
// There is one fuzzy matcher in the program, so ranking feels the same in the
// palette, the `/` filter and every picker — and one palette implementation, so
// there is no second place for the list of what pogo can do to drift.
//
// Every row carries its keyboard shortcut, because finding something by
// searching should teach the key for next time. The palette is meant to make
// itself unnecessary.

// paletteCommands projects the registry onto the kit's command type. Commands
// that cannot run right now are listed dimmed with the reason, rather than
// hidden: a palette that hides things teaches that they do not exist.
func (m *Model) paletteCommands() []ui.PaletteCommand {
	items := m.paletteItems()
	out := make([]ui.PaletteCommand, 0, len(items))
	for _, c := range items {
		cmd := ui.PaletteCommand{
			ID:       c.id,
			Title:    c.title,
			Group:    c.group,
			Shortcut: c.keys,
		}
		if !c.available(m) {
			cmd.Disabled = true
			cmd.Reason = "needs a request selected"
		}
		out = append(out, cmd)
	}
	return out
}

// openPalette rebuilds the palette against what is available right now and
// hands it the keyboard.
func (m *Model) openPalette() {
	m.palette = ui.NewPalette("Commands", m.paletteCommands()).Show()
	m.overlay = overlayPalette
}

// handlePaletteKey feeds one key to the palette and runs whatever it returns.
func (m *Model) handlePaletteKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	var chosen string
	m.palette, chosen = m.palette.Update(msg)

	if !m.palette.Open() {
		m.overlay = overlayNone
	}
	if chosen == "" {
		return true, nil
	}
	for _, c := range m.commands() {
		if c.id == chosen && c.run != nil {
			return true, c.run(m)
		}
	}
	return true, nil
}

func (m *Model) renderPalette(width, height int) string {
	return m.palette.View(width, height)
}

// paletteTopMatch reports the id the palette would run right now. It exists
// for the tests: the palette owns its own ranking, and asserting on a rendered
// frame to check an ordering is a test of the wrong thing.
func (m *Model) paletteTopMatch() string {
	return m.palette.TopMatch()
}
