package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/rmpato/pogo/internal/ui"
)

// Every style pogo draws with is built here, from the theme tokens in
// internal/ui and from nothing else. No screen constructs a colour of its own
// (whis SYSTEM_DESIGN.md §4.3): a palette that lives in thirty files is a
// palette nobody can change.
//
// Colour in pogo carries meaning — what kind of request this was, whether it
// succeeded, what is selected — and nothing else. Anything decorative is left
// in the terminal's own foreground so the tool inherits the user's theme
// instead of fighting it.
var (
	styText  lipgloss.Style
	styMuted lipgloss.Style
	styFaint lipgloss.Style
	styRule  lipgloss.Style

	styTitle    lipgloss.Style
	styHeading  lipgloss.Style
	stySelected lipgloss.Style
	styCursor   lipgloss.Style
	styStar     lipgloss.Style
	styErr      lipgloss.Style
	styOK       lipgloss.Style
	styKey      lipgloss.Style
	styKeyHint  lipgloss.Style
	styBadge    lipgloss.Style

	styAccentBar lipgloss.Style
	styDiffAdd   lipgloss.Style
	styDiffDel   lipgloss.Style
	styYellowDot string
)

func init() { refreshStyles() }

// refreshStyles rebuilds every derived style from the current theme. It is the
// counterpart of ui.ApplyTheme: switching theme reassigns the tokens, and this
// reassigns everything built out of them, before the next frame is drawn.
func refreshStyles() {
	styText = lipgloss.NewStyle().Foreground(ui.Text)
	styMuted = lipgloss.NewStyle().Foreground(ui.Muted)
	styFaint = lipgloss.NewStyle().Foreground(ui.Border)
	styRule = lipgloss.NewStyle().Foreground(ui.Border)

	styTitle = lipgloss.NewStyle().Foreground(ui.Primary).Bold(true)
	styHeading = lipgloss.NewStyle().Foreground(ui.Muted).Bold(true)
	stySelected = lipgloss.NewStyle().Foreground(ui.Text).Bold(true)
	styCursor = lipgloss.NewStyle().Foreground(ui.Primary).Bold(true)
	styStar = lipgloss.NewStyle().Foreground(ui.Warning)
	styErr = lipgloss.NewStyle().Foreground(ui.Danger)
	styOK = lipgloss.NewStyle().Foreground(ui.Success)
	styKey = lipgloss.NewStyle().Foreground(ui.Text).Bold(true)
	styKeyHint = lipgloss.NewStyle().Foreground(ui.Muted)
	styBadge = lipgloss.NewStyle().Foreground(ui.Alt)

	styJSONKey = lipgloss.NewStyle().Foreground(ui.Primary)
	styJSONString = lipgloss.NewStyle().Foreground(ui.Success)
	styJSONNumber = lipgloss.NewStyle().Foreground(ui.Warning)
	styJSONBool = lipgloss.NewStyle().Foreground(ui.Alt)
	styJSONNull = lipgloss.NewStyle().Foreground(ui.Muted)
	styJSONPunct = lipgloss.NewStyle().Foreground(ui.Border)

	styAccentBar = lipgloss.NewStyle().Foreground(ui.Primary)
	styDiffAdd = lipgloss.NewStyle().Foreground(ui.Success)
	styDiffDel = lipgloss.NewStyle().Foreground(ui.Danger)
	styYellowDot = lipgloss.NewStyle().Foreground(ui.Warning).Render("• ")
}

// methodStyle colors an HTTP method by how much damage it can do: reads are
// calm, writes are warm, deletes are loud.
func methodStyle(method string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(ui.MethodColor(method))
}

// statusStyle colors a response by class. A request that never produced a
// status at all is an error, and reads as one.
func statusStyle(code int) lipgloss.Style { return ui.StatusStyle(code) }

// rule draws a horizontal separator. Single lines and whitespace do the work
// that boxes would otherwise do, which keeps dense screens readable.
func rule(width int) string {
	if width <= 0 {
		return ""
	}
	return styRule.Render(repeat("─", width))
}

func repeat(s string, n int) string {
	if n <= 0 {
		return ""
	}
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
