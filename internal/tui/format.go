package tui

import (
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// age renders how long ago something happened, in the shortest form that is
// still unambiguous. History is scanned, not read, so "2m" beats "2 minutes
// ago" and a fixed width keeps the column from jittering.
func age(t time.Time, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < 0:
		return "now"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	default:
		return t.Local().Format("Jan 2")
	}
}

// bytesHuman formats a size for a dense column.
func bytesHuman(n int64) string {
	switch {
	case n <= 0:
		return "—"
	case n < 1024:
		return fmt.Sprintf("%dB", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1fK", float64(n)/1024)
	case n < 1024*1024*1024:
		return fmt.Sprintf("%.1fM", float64(n)/(1024*1024))
	default:
		return fmt.Sprintf("%.1fG", float64(n)/(1024*1024*1024))
	}
}

// truncate shortens text to width with an ellipsis.
func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	runes := []rune(s)
	return string(runes[:width-1]) + "…"
}

// truncateMiddle keeps both ends of a string, which is what a long URL needs:
// the host at the front and the interesting part of the path at the end are
// both load-bearing, and the middle rarely is.
func truncateMiddle(s string, width int) string {
	n := utf8.RuneCountInString(s)
	if width <= 0 || n <= width {
		if width <= 0 {
			return ""
		}
		return s
	}
	if width <= 3 {
		return truncate(s, width)
	}
	runes := []rune(s)
	keep := width - 1
	head := keep - keep/2
	tail := keep / 2
	return string(runes[:head]) + "…" + string(runes[n-tail:])
}

// pad right-pads to an exact display width, and truncates when too long, so
// callers can build fixed columns without measuring twice.
func pad(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = truncate(s, width)
	if gap := width - lipgloss.Width(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return s
}

// padLeft right-aligns within a fixed column.
func padLeft(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = truncate(s, width)
	if gap := width - lipgloss.Width(s); gap > 0 {
		return strings.Repeat(" ", gap) + s
	}
	return s
}

// statusText renders a status code, or the reason a request has none.
func statusText(code, exit int) string {
	if code > 0 {
		return fmt.Sprintf("%d", code)
	}
	if exit != 0 {
		return "ERR"
	}
	return "—"
}

// clampLine cuts a rendered line to an exact display width without breaking the
// ANSI sequences inside it. Styled text measured naively would be cut mid-escape
// and leak color across the rest of the screen.
func clampLine(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	return ansi.Truncate(s, width, "")
}

// clampBlock clamps every line of a block, which keeps viewport content inside
// its pane no matter what a server returned.
func clampBlock(s string, width int) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = clampLine(l, width)
	}
	return strings.Join(lines, "\n")
}

// fitHeight makes a block exactly height lines tall.
//
// Every screen is composed inside a fixed area between the header and the
// footer, and both kinds of mismatch are visible: too tall shifts the frame and
// scrolls the header out of view, too short leaves the footer floating in the
// middle of the screen. Padding and truncating here means no screen has to
// remember to do it.
func fitHeight(s string, height int) string {
	if height <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) > height {
		return strings.Join(lines[:height], "\n")
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// homeDir is a variable so tests can pin it without touching the environment.
var homeDir = os.UserHomeDir

// lineWidth reports the display width of a rendered line, ignoring styling.
func lineWidth(s string) int { return lipgloss.Width(s) }
