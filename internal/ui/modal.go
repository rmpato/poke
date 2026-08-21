package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// ---------------------------------------------------------------------------
// Modals
// ---------------------------------------------------------------------------
//
// Per the modal-stack rule, only one modal is ever open at a time, and the
// screen underneath keeps rendering behind it. These helpers all return a
// full terminal rectangle with the modal placed dead centre, so a screen's
// View() can return one directly.

// ModalWidth picks a modal width that stays readable on a wide terminal and
// still fits a narrow one. Screens should use this rather than inventing
// their own arithmetic, so every modal in the app lines up.
func ModalWidth(width int) int {
	return min(max(46, width-14), 80)
}

// Modal centres a bordered box of body over a width x height rectangle.
// body is clamped to the inner area, so callers can hand it more content
// than fits without breaking the frame.
func Modal(title, body string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	outer := ModalWidth(width)
	inner := max(24, outer-6)
	innerHeight := max(4, min(height-8, lipgloss.Height(body)+2))

	content := body
	if title != "" {
		content = TitleStyle.Render(title) + "\n\n" + body
		innerHeight = min(max(6, height-8), lipgloss.Height(content))
	}

	box := ModalStyle.Render(ClampBlock(content, inner, innerHeight))
	return ClampBlock(lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box), width, height)
}

// HelpSection is one titled group of key/description rows in a help modal.
type HelpSection struct {
	Title string
	Rows  [][2]string
}

// HelpModal renders the `?` overlay: sections of key/description pairs with
// the keys aligned into a column, plus the close hint every modal carries.
//
// Every screen still writes its own sections — "what does `a` do here" is
// only answerable in context — but none of them should be re-deriving the
// layout.
func HelpModal(title string, sections []HelpSection, width, height int) string {
	keyWidth := 0
	for _, section := range sections {
		for _, row := range section.Rows {
			keyWidth = max(keyWidth, lipgloss.Width(row[0]))
		}
	}

	blocks := make([]string, 0, len(sections)*2+1)
	for _, section := range sections {
		if section.Title != "" {
			blocks = append(blocks, BrandStyle.Render(section.Title))
		}
		for _, row := range section.Rows {
			key := ValueStyle.Render(alignCell(row[0], keyWidth, AlignLeft))
			blocks = append(blocks, "  "+key+"   "+SubtitleStyle.Render(row[1]))
		}
		blocks = append(blocks, "")
	}
	blocks = append(blocks, SubtitleStyle.Render(Keycap("?")+"/"+Keycap("esc")+" close"))

	return Modal(title, strings.Join(blocks, "\n"), width, height)
}

// ConfirmModal renders an in-TUI yes/no prompt with the two choices as
// buttons. Use this when the confirmation belongs to the screen you're
// already on; use a Huh form (outside the Bubble Tea loop) when the
// confirmation is really a small wizard of its own.
//
// Destructive confirmations should default to the cancelling choice —
// affirmative false — so a reflex Enter is safe.
func ConfirmModal(title, body, affirmative, negative string, affirmativeFocused bool, width, height int) string {
	focused := lipgloss.NewStyle().Foreground(PrimaryFg).Background(Primary).Bold(true)
	blurred := lipgloss.NewStyle().Foreground(Muted).Background(Surface)

	yes, no := blurred.Render(" "+affirmative+" "), blurred.Render(" "+negative+" ")
	if affirmativeFocused {
		yes = focused.Render(" " + affirmative + " ")
	} else {
		no = focused.Render(" " + negative + " ")
	}

	content := strings.Join([]string{
		body,
		"",
		yes + "  " + no,
		"",
		SubtitleStyle.Render(Keycap("←") + "/" + Keycap("→") + " choose · " + Keycap("enter") + " confirm · " + Keycap("esc") + " cancel"),
	}, "\n")
	return Modal(title, content, width, height)
}

// Overlay places any pre-rendered block centred over a full rectangle,
// for a modal you've built yourself.
func Overlay(box string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	return ClampBlock(lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box), width, height)
}

// OverlayOn composites a pre-rendered box onto an already-rendered frame,
// centred, replacing the rows it covers.
//
// Modal and Overlay place a box on an empty rectangle, which is what a screen
// that is *only* a dialog wants. This is for the other case the modal-stack
// rule describes: the screen underneath keeps rendering behind the dialog, so
// closing it does not feel like arriving somewhere new. frame must already be
// an exact width x height rectangle; the result is too.
func OverlayOn(frame, box string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	if box == "" {
		return frame
	}

	rows := strings.Split(ClampBlock(frame, width, height), "\n")
	boxLines := strings.Split(box, "\n")

	boxWidth := 0
	for _, line := range boxLines {
		boxWidth = max(boxWidth, lipgloss.Width(line))
	}
	left := max(0, (width-boxWidth)/2)
	top := max(0, (height-len(boxLines))/2)

	for index, line := range boxLines {
		at := top + index
		if at < 0 || at >= len(rows) {
			continue
		}
		// The row is rebuilt from the frame's own left edge, so whatever the
		// box does not cover keeps showing through.
		prefix := ansi.Truncate(rows[at], left, "")
		if pad := left - ansi.StringWidth(prefix); pad > 0 {
			prefix += strings.Repeat(" ", pad)
		}
		rows[at] = FitLine(prefix+line, width)
	}
	return strings.Join(rows, "\n")
}
