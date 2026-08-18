package tui

import "github.com/charmbracelet/lipgloss"

// The palette is deliberately small. Color in pogo carries meaning -- what
// kind of request this was, whether it succeeded, what is selected -- and
// nothing else. Anything decorative is rendered in the terminal's own
// foreground color so the tool inherits the user's theme instead of fighting
// it.
var (
	colText   = lipgloss.AdaptiveColor{Light: "236", Dark: "252"}
	colMuted  = lipgloss.AdaptiveColor{Light: "245", Dark: "244"}
	colFaint  = lipgloss.AdaptiveColor{Light: "250", Dark: "239"}
	colRule   = lipgloss.AdaptiveColor{Light: "252", Dark: "237"}
	colAccent = lipgloss.AdaptiveColor{Light: "25", Dark: "39"}

	colGreen  = lipgloss.AdaptiveColor{Light: "28", Dark: "42"}
	colBlue   = lipgloss.AdaptiveColor{Light: "25", Dark: "39"}
	colYellow = lipgloss.AdaptiveColor{Light: "130", Dark: "214"}
	colRed    = lipgloss.AdaptiveColor{Light: "160", Dark: "203"}
	colPurple = lipgloss.AdaptiveColor{Light: "91", Dark: "141"}
	colStar   = lipgloss.AdaptiveColor{Light: "136", Dark: "220"}
)

var (
	styText  = lipgloss.NewStyle().Foreground(colText)
	styMuted = lipgloss.NewStyle().Foreground(colMuted)
	styFaint = lipgloss.NewStyle().Foreground(colFaint)
	styRule  = lipgloss.NewStyle().Foreground(colRule)

	styTitle    = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	styHeading  = lipgloss.NewStyle().Foreground(colMuted).Bold(true)
	stySelected = lipgloss.NewStyle().Foreground(colText).Bold(true)
	styCursor   = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	styStar     = lipgloss.NewStyle().Foreground(colStar)
	styErr      = lipgloss.NewStyle().Foreground(colRed)
	styOK       = lipgloss.NewStyle().Foreground(colGreen)
	styKey      = lipgloss.NewStyle().Foreground(colText).Bold(true)
	styKeyHint  = lipgloss.NewStyle().Foreground(colMuted)
	styBadge    = lipgloss.NewStyle().Foreground(colPurple)
)

// methodStyle colors an HTTP method by how much damage it can do: reads are
// calm, writes are warm, deletes are loud.
func methodStyle(method string) lipgloss.Style {
	switch method {
	case "GET":
		return lipgloss.NewStyle().Foreground(colBlue)
	case "POST":
		return lipgloss.NewStyle().Foreground(colGreen)
	case "PUT", "PATCH":
		return lipgloss.NewStyle().Foreground(colYellow)
	case "DELETE":
		return lipgloss.NewStyle().Foreground(colRed)
	case "HEAD", "OPTIONS":
		return lipgloss.NewStyle().Foreground(colMuted)
	default:
		return lipgloss.NewStyle().Foreground(colPurple)
	}
}

// statusStyle colors a response by class. A request that never produced a
// status at all is an error, and reads as one.
func statusStyle(code int) lipgloss.Style {
	switch {
	case code == 0:
		return lipgloss.NewStyle().Foreground(colRed)
	case code < 200:
		return lipgloss.NewStyle().Foreground(colMuted)
	case code < 300:
		return lipgloss.NewStyle().Foreground(colGreen)
	case code < 400:
		return lipgloss.NewStyle().Foreground(colBlue)
	case code < 500:
		return lipgloss.NewStyle().Foreground(colYellow)
	default:
		return lipgloss.NewStyle().Foreground(colRed)
	}
}

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

// Styles used by the timing bars and the diff view.
var (
	styAccentBar = lipgloss.NewStyle().Foreground(colAccent)
	styDiffAdd   = lipgloss.NewStyle().Foreground(colGreen)
	styDiffDel   = lipgloss.NewStyle().Foreground(colRed)
	styYellowDot = lipgloss.NewStyle().Foreground(colYellow).Render("• ")
)
