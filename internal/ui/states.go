package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// ---------------------------------------------------------------------------
// Severity
// ---------------------------------------------------------------------------

// Kind is the severity shared by banners, toasts and step lists, so "this
// went wrong" looks identical wherever it surfaces.
type Kind int

const (
	KindInfo Kind = iota
	KindSuccess
	KindWarning
	KindDanger
)

// Color returns the theme token for a severity.
func (k Kind) Color() lipgloss.TerminalColor {
	switch k {
	case KindSuccess:
		return Success
	case KindWarning:
		return Warning
	case KindDanger:
		return Danger
	default:
		return Primary
	}
}

// Glyph returns the one-cell marker for a severity. These are the same four
// glyphs Info/Ok/Warn/Fail print on the CLI side, which is what makes piped
// output and the TUI read as one tool.
func (k Kind) Glyph() string {
	switch k {
	case KindSuccess:
		return "✓"
	case KindWarning:
		return "!"
	case KindDanger:
		return "✗"
	default:
		return "→"
	}
}

// ---------------------------------------------------------------------------
// Bars and chrome
// ---------------------------------------------------------------------------

// StatusBar renders a full-width row with left- and right-aligned halves.
// When the two can't both fit, the right half wins — it's usually the key
// hint, and a user who can't see the keys is stuck, whereas one who can't
// see the breadcrumb is merely lost.
func StatusBar(left, right string, width int) string {
	if width <= 0 {
		return ""
	}
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		if lipgloss.Width(right) >= width {
			return FitLine(right, width)
		}
		left = ansi.Truncate(left, max(0, width-lipgloss.Width(right)-1), "…")
		gap = max(1, width-lipgloss.Width(left)-lipgloss.Width(right))
	}
	return FitLine(left+strings.Repeat(" ", gap)+right, width)
}

// Breadcrumb renders a navigation trail, dimming everything but the last
// segment. Long trails drop leading segments to an ellipsis rather than
// truncating the segment the user is actually in.
func Breadcrumb(parts []string, width int) string {
	if width <= 0 || len(parts) == 0 {
		return ""
	}
	separator := lipgloss.NewStyle().Foreground(Border).Render(" › ")

	build := func(items []string, elided bool) string {
		rendered := make([]string, 0, len(items)+1)
		if elided {
			rendered = append(rendered, SubtitleStyle.Render("…"))
		}
		for index, part := range items {
			if index == len(items)-1 {
				rendered = append(rendered, ValueStyle.Render(part))
				continue
			}
			rendered = append(rendered, SubtitleStyle.Render(part))
		}
		return strings.Join(rendered, separator)
	}

	for drop := 0; drop < len(parts)-1; drop++ {
		candidate := build(parts[drop:], drop > 0)
		if lipgloss.Width(candidate) <= width {
			return FitLine(candidate, width)
		}
	}
	return FitLine(ValueStyle.Render(parts[len(parts)-1]), width)
}

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

// Banner renders a bordered, severity-colored callout for something the
// user must read before continuing — a failed action, a destructive
// warning. Use Toast for things they can ignore.
func Banner(kind Kind, title, body string, width int) string {
	if width <= 0 {
		return ""
	}
	accent := kind.Color()
	bar := lipgloss.NewStyle().Foreground(accent).Render("┃")
	head := lipgloss.NewStyle().Foreground(accent).Bold(true).Render(kind.Glyph() + " " + title)

	lines := []string{FitLine(bar+" "+head, width)}
	if body != "" {
		for _, line := range strings.Split(ansi.Wordwrap(body, max(1, width-2), " "), "\n") {
			lines = append(lines, FitLine(bar+" "+SubtitleStyle.Render(line), width))
		}
	}
	return strings.Join(lines, "\n")
}

// Toast renders a single-line transient notice. Screens usually place it
// with lipgloss.Place near the bottom of the frame and clear it on the next
// keypress — it should never be the only place important information
// appears.
func Toast(kind Kind, message string, width int) string {
	accent := kind.Color()
	label := lipgloss.NewStyle().
		Foreground(PrimaryFg).
		Background(accent).
		Bold(true).
		Render(" " + kind.Glyph() + " ")
	body := lipgloss.NewStyle().
		Foreground(Text).
		Background(Surface).
		Render(" " + ansi.Truncate(message, max(4, width-6), "…") + " ")
	return label + body
}

// EmptyState fills a rectangle with a centered glyph, title and hint. Always
// give it a hint that names the key or command that would populate the
// screen — an empty list with no way forward reads as broken.
func EmptyState(glyph, title, hint string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	block := strings.Join([]string{
		lipgloss.NewStyle().Foreground(Border).Render(glyph),
		"",
		ValueStyle.Render(title),
		SubtitleStyle.Render(hint),
	}, "\n")
	centered := lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(block)
	return ClampBlock(lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, centered), width, height)
}

// ---------------------------------------------------------------------------
// Progress
// ---------------------------------------------------------------------------

// StepState is where one step of a multi-step operation has got to.
type StepState int

const (
	StepPending StepState = iota
	StepActive
	StepDone
	StepFailed
	StepSkipped
)

// ProgressStep is one line of a StepList.
type ProgressStep struct {
	Title  string
	Detail string
	State  StepState
}

// StepList renders a checklist for a long-running operation: what's done,
// what's happening now, what's still queued. Prefer this to a percentage
// bar whenever the steps have names — "publishing manifest" tells a user
// far more than "62%".
//
// frame animates the active step's marker; pass a counter that increments
// on your spinner tick, or 0 for a static render.
func StepList(steps []ProgressStep, frame, width int) string {
	if width <= 0 || len(steps) == 0 {
		return ""
	}
	spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

	lines := make([]string, 0, len(steps))
	for _, step := range steps {
		var marker, title string
		switch step.State {
		case StepDone:
			marker = SuccessStyle.Render("✓")
			title = SubtitleStyle.Render(step.Title)
		case StepActive:
			marker = BrandStyle.Render(spinner[((frame%len(spinner))+len(spinner))%len(spinner)])
			title = ValueStyle.Render(step.Title)
		case StepFailed:
			marker = ErrorStyle.Render("✗")
			title = ErrorStyle.Render(step.Title)
		case StepSkipped:
			marker = lipgloss.NewStyle().Foreground(Border).Render("–")
			title = lipgloss.NewStyle().Foreground(Border).Render(step.Title)
		default:
			marker = lipgloss.NewStyle().Foreground(Border).Render("○")
			title = SubtitleStyle.Render(step.Title)
		}

		line := marker + " " + title
		if step.Detail != "" {
			line += "  " + SubtitleStyle.Render(step.Detail)
		}
		lines = append(lines, FitLine(line, width))
	}
	return strings.Join(lines, "\n")
}

// Swatch renders a labeled color chip, for a theme reference screen or a
// palette picker.
func Swatch(label string, color lipgloss.TerminalColor, width int) string {
	chip := lipgloss.NewStyle().Background(color).Render("    ")
	return FitLine(chip+" "+LabelStyle.Render(label), width)
}

// ---------------------------------------------------------------------------
// Ranking
// ---------------------------------------------------------------------------

// PodiumColor gives ranks 0/1/2 fixed gold/silver/bronze accents,
// independent of the active theme — a leaderboard that repaints with the
// palette loses the one association users already have.
func PodiumColor(rank int) lipgloss.TerminalColor {
	switch rank {
	case 0:
		return lipgloss.Color("#E8B923")
	case 1:
		return lipgloss.Color("#B9C1D9")
	case 2:
		return lipgloss.Color("#C97B3C")
	default:
		return Muted
	}
}

// PodiumStyle is PodiumColor as a style, bold for the top three.
func PodiumStyle(rank int) lipgloss.Style {
	style := lipgloss.NewStyle().Foreground(PodiumColor(rank))
	if rank < 3 {
		return style.Bold(true)
	}
	return style
}
