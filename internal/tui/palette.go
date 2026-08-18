package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type (
	teaKeyMsg = tea.KeyMsg
	teaCmd    = tea.Cmd
)

// paletteHeight is how many commands are listed at once. Enough to browse,
// short enough that the answer is usually on screen without scrolling.
const paletteHeight = 9

// renderPalette draws the command palette.
//
// Every row carries its keyboard shortcut, so finding something by searching
// also teaches the key for next time. The palette is meant to become
// unnecessary.
func (m *Model) renderPalette(width, height int) string {
	items := m.filterCommands(m.paletteInput.Value())

	boxWidth := clampInt(width-8, 40, 72)
	// The border style adds two columns of padding either side, so rows are
	// built to the inner width; measuring against the outer one wraps every
	// right-aligned key onto its own line.
	inner := boxWidth - 4

	var b strings.Builder

	b.WriteString(styHeading.Render("COMMANDS") + "\n")
	m.paletteInput.Width = inner
	b.WriteString(m.paletteInput.View() + "\n\n")

	if len(items) == 0 {
		b.WriteString(styFaint.Render("  nothing matches " + m.paletteInput.Value()))
	}

	// Scroll the window so the cursor stays visible in a long list.
	start := 0
	if m.paletteCursor >= paletteHeight {
		start = m.paletteCursor - paletteHeight + 1
	}
	end := minInt(start+paletteHeight, len(items))

	for i := start; i < end; i++ {
		c := items[i]
		selected := i == m.paletteCursor
		enabled := c.available(m)

		cursor := "  "
		title := styText.Render(c.title)
		desc := styFaint.Render(c.desc)

		switch {
		case selected:
			cursor = styCursor.Render("▌ ")
			title = stySelected.Render(c.title)
			desc = styMuted.Render(c.desc)
		case !enabled:
			title = styFaint.Render(c.title)
		}
		if !enabled {
			desc = styFaint.Render("needs a request selected")
		}

		key := ""
		if c.keys != "" {
			key = styKey.Render(c.keys)
		}

		left := cursor + title
		gap := inner - lipgloss.Width(left) - lipgloss.Width(key)
		if gap < 1 {
			gap = 1
		}
		b.WriteString(clampLine(left+strings.Repeat(" ", gap)+key, inner) + "\n")

		if c.desc != "" && selected {
			b.WriteString(clampLine("    "+desc, inner) + "\n")
		}
	}

	if len(items) > end {
		b.WriteString(styFaint.Render("  … " + itoa(len(items)-end) + " more"))
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colAccent).
		Padding(0, 2).
		Width(boxWidth).
		Render(b.String())

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

// handlePaletteKey drives the palette: type to filter, arrows to choose, enter
// to run.
func (m *Model) handlePaletteKey(msg teaKeyMsg) (handled bool, cmd teaCmd) {
	items := m.filterCommands(m.paletteInput.Value())

	switch msg.String() {
	case "esc", "ctrl+k":
		m.overlay = overlayNone
		m.paletteInput.Blur()
		return true, nil

	case "down", "ctrl+n":
		if len(items) > 0 {
			m.paletteCursor = clampInt(m.paletteCursor+1, 0, len(items)-1)
		}
		return true, nil

	case "up", "ctrl+p":
		if len(items) > 0 {
			m.paletteCursor = clampInt(m.paletteCursor-1, 0, len(items)-1)
		}
		return true, nil

	case "enter":
		m.overlay = overlayNone
		m.paletteInput.Blur()
		if m.paletteCursor < len(items) {
			c := items[m.paletteCursor]
			if !c.available(m) {
				m.flashErr(c.title + " needs a request selected")
				return true, clearStatus(m.statusTok)
			}
			if c.run != nil {
				return true, c.run(m)
			}
		}
		return true, nil
	}

	var inputCmd teaCmd
	m.paletteInput, inputCmd = m.paletteInput.Update(msg)

	// Re-filtering can shorten the list under the cursor.
	if n := len(m.filterCommands(m.paletteInput.Value())); m.paletteCursor >= n {
		m.paletteCursor = maxInt(0, n-1)
	}
	return true, inputCmd
}
