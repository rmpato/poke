package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Command palette
// ---------------------------------------------------------------------------
//
// The one component here that holds state, because a palette is a mode:
// it owns the keyboard while it's open. It's a plain value embedded in a
// screen's model rather than its own tea.Model, so a screen keeps a single
// Update/View and doesn't have to nest programs.
//
// Typical wiring:
//
//	case tea.KeyMsg:
//	    if m.palette.Open() {
//	        var chosen string
//	        m.palette, chosen = m.palette.Update(msg)
//	        if chosen != "" { return m.run(chosen) }
//	        return m, nil
//	    }
//	    if msg.String() == "ctrl+k" { m.palette = m.palette.Show(); return m, nil }

// PaletteCommand is one entry in the palette. Group headers entries in the
// list; Shortcut is advertised on the right so the palette teaches the
// direct key rather than replacing it.
type PaletteCommand struct {
	ID       string
	Title    string
	Group    string
	Shortcut string
}

// Palette is an embeddable fuzzy command launcher.
type Palette struct {
	commands []PaletteCommand
	title    string
	query    string
	cursor   int
	matches  []int
	open     bool
}

// NewPalette builds a palette over a fixed command set.
func NewPalette(title string, commands []PaletteCommand) Palette {
	p := Palette{commands: commands, title: Fallback(title, "Commands")}
	return p.refilter()
}

// Open reports whether the palette currently owns the keyboard.
func (p Palette) Open() bool { return p.open }

// Show opens the palette with a cleared query.
func (p Palette) Show() Palette {
	p.open = true
	p.query = ""
	p.cursor = 0
	return p.refilter()
}

// Hide closes the palette without choosing anything.
func (p Palette) Hide() Palette {
	p.open = false
	return p
}

// Update feeds one key to the palette. The returned string is the ID of a
// chosen command, or empty if nothing was chosen this keypress. The palette
// closes itself on choose and on esc.
func (p Palette) Update(msg tea.KeyMsg) (Palette, string) {
	switch msg.String() {
	case "esc", "ctrl+c", "ctrl+k":
		return p.Hide(), ""
	case "enter":
		if p.cursor < len(p.matches) {
			id := p.commands[p.matches[p.cursor]].ID
			return p.Hide(), id
		}
		return p, ""
	case "up", "ctrl+p":
		if p.cursor > 0 {
			p.cursor--
		}
		return p, ""
	case "down", "ctrl+n", "tab":
		if p.cursor < len(p.matches)-1 {
			p.cursor++
		}
		return p, ""
	case "backspace":
		if runes := []rune(p.query); len(runes) > 0 {
			p.query = string(runes[:len(runes)-1])
		}
		return p.refilter(), ""
	}

	if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 {
		p.query += string(msg.Runes)
		return p.refilter(), ""
	}
	if msg.Type == tea.KeySpace {
		p.query += " "
		return p.refilter(), ""
	}
	return p, ""
}

// refilter re-ranks against the current query, keeping the cursor in range.
func (p Palette) refilter() Palette {
	haystack := make([]string, len(p.commands))
	for index, command := range p.commands {
		// Rank against group + title so "set theme" finds Settings → Theme.
		haystack[index] = strings.TrimSpace(command.Group + " " + command.Title)
	}
	p.matches = FuzzyRank(p.query, haystack)
	if p.cursor >= len(p.matches) {
		p.cursor = max(0, len(p.matches)-1)
	}
	return p
}

// View renders the palette over a full terminal rectangle. Call it only
// when Open() is true; the screen underneath should render normally
// otherwise.
func (p Palette) View(width, height int) string {
	outer := ModalWidth(width)
	inner := max(24, outer-6)
	rows := max(3, min(12, height-12))

	prompt := BrandStyle.Render("❯ ") + ValueStyle.Render(p.query) + BrandStyle.Render("▏")
	head := StatusBar(prompt, SubtitleStyle.Render(countLabel(len(p.matches), len(p.commands))), inner)
	rule := lipgloss.NewStyle().Foreground(Border).Render(strings.Repeat("─", inner))

	body := p.renderList(inner, rows)
	footer := SubtitleStyle.Render(
		Keycap("↑↓") + " move · " + Keycap("enter") + " run · " + Keycap("esc") + " close")

	content := strings.Join([]string{
		TitleStyle.Render(p.title), "", head, rule, body, "", footer,
	}, "\n")

	box := ModalStyle.Render(ClampBlock(content, inner, lipgloss.Height(content)))
	return Overlay(box, width, height)
}

func countLabel(shown, total int) string {
	if shown == total {
		return ""
	}
	return fmt.Sprintf("%d/%d", shown, total)
}

func (p Palette) renderList(width, height int) string {
	if len(p.matches) == 0 {
		return ClampBlock(SubtitleStyle.Render("  no matching command"), width, height)
	}

	start, end := Window(p.cursor, height, len(p.matches))
	lines := make([]string, 0, height)

	for index := start; index < end; index++ {
		command := p.commands[p.matches[index]]
		selected := index == p.cursor

		label := command.Title
		if command.Group != "" {
			label = command.Group + " › " + command.Title
		}

		if selected {
			// Plain text only inside a selected row: an inner colour would
			// end the highlight background partway along the line.
			left := "  " + label
			right := command.Shortcut + " "
			lines = append(lines, SelectedRowStyle.Render(StatusBar(left, right, width)))
			continue
		}

		left := "  " + HighlightMatch(p.query, label, lipgloss.NewStyle().Foreground(Text))
		right := SubtitleStyle.Render(command.Shortcut + " ")
		lines = append(lines, StatusBar(left, right, width))
	}
	return ClampBlock(strings.Join(lines, "\n"), width, height)
}
