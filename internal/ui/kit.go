package ui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// This file is the shared component kit: small, composable renderers every
// screen uses so the app stays visually consistent without each screen
// inventing its own bars/badges/panels.
//
// Two rules hold everywhere:
//
//  1. Renderers return blocks of an exact cell width, so a caller can join
//     them horizontally without the layout drifting.
//  2. Styling always closes before end of line. A style left open would
//     bleed into the rest of the terminal row on some terminals.
//
// Renderers whose output may be nested inside an already-styled line (a
// selected table row, for example) come in a "Plain" variant that emits
// glyphs only, because a nested style's reset would punch a hole in the
// outer highlight.

// Bar glyph ramps. Eighth blocks give a meter sub-cell precision; the shade
// ramp is used where a softer, lower-contrast fill reads better.
const (
	barFull    = "█"
	barPartial = " ▏▎▍▌▋▊▉"
	barEmpty   = "░"

	sparkRamp = "▁▂▃▄▅▆▇█"
)

// ---------------------------------------------------------------------------
// Colour maths
// ---------------------------------------------------------------------------

func hexToRGB(hex string) (float64, float64, float64) {
	hex = strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}
	if len(hex) != 6 {
		return 0, 0, 0
	}
	var r, g, b int
	if _, err := fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b); err != nil {
		return 0, 0, 0
	}
	return float64(r), float64(g), float64(b)
}

// blendChannel interpolates in linear light rather than raw sRGB, which keeps
// mid-gradient colours from muddying into grey.
func blendChannel(from, to, t float64) float64 {
	const gamma = 2.2
	a := math.Pow(from/255, gamma)
	b := math.Pow(to/255, gamma)
	return math.Pow(a+(b-a)*t, 1/gamma) * 255
}

// BlendHex mixes two hex colours; t is clamped to [0,1].
func BlendHex(from, to string, t float64) string {
	t = clampUnit(t)
	fr, fg, fb := hexToRGB(from)
	tr, tg, tb := hexToRGB(to)
	return fmt.Sprintf("#%02X%02X%02X",
		int(math.Round(blendChannel(fr, tr, t))),
		int(math.Round(blendChannel(fg, tg, t))),
		int(math.Round(blendChannel(fb, tb, t))),
	)
}

func clampUnit(value float64) float64 {
	if math.IsNaN(value) || value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

// ---------------------------------------------------------------------------
// Gradient text
// ---------------------------------------------------------------------------

// Gradient paints text across a colour ramp, one cell at a time. Whitespace
// is left unstyled so runs of spaces cost nothing.
func Gradient(text, from, to string) string {
	return gradientWith(text, from, to, false)
}

// GradientBold is Gradient with weight, for wordmarks and headline numbers.
func GradientBold(text, from, to string) string {
	return gradientWith(text, from, to, true)
}

// GradientBrand paints text with the active theme's accent ramp.
func GradientBrand(text string) string {
	return GradientBold(text, GradientFrom, GradientTo)
}

func gradientWith(text, from, to string, bold bool) string {
	runes := []rune(text)
	if len(runes) == 0 {
		return ""
	}
	if len(runes) == 1 {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(from)).Bold(bold)
		return style.Render(text)
	}
	var out strings.Builder
	span := float64(len(runes) - 1)
	for index, glyph := range runes {
		if glyph == ' ' {
			out.WriteRune(glyph)
			continue
		}
		color := BlendHex(from, to, float64(index)/span)
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(bold)
		out.WriteString(style.Render(string(glyph)))
	}
	return out.String()
}

// ---------------------------------------------------------------------------
// Wordmark
// ---------------------------------------------------------------------------

// Wordmark renders name as a letter-spaced, gradient-filled logotype when
// there's room, falling back to a tight rendering and finally a two-letter
// monogram as width shrinks. name should be short (a product name, not a
// tagline) — this is meant for a header, not body copy.
func Wordmark(name string, width int) string {
	spaced := spaceOutLetters(name)
	switch {
	case width >= lipgloss.Width(spaced)+6:
		return GradientBrand(spaced)
	case width >= len(name)+2:
		return GradientBrand(strings.ToUpper(name))
	default:
		return BrandStyle.Render(monogram(name))
	}
}

func spaceOutLetters(name string) string {
	letters := strings.Split(strings.ToUpper(name), "")
	return strings.Join(letters, " ")
}

func monogram(name string) string {
	fields := strings.FieldsFunc(name, func(r rune) bool {
		return r == '-' || r == '_' || r == ' '
	})
	if len(fields) >= 2 {
		return strings.ToUpper(fields[0][:1] + fields[1][:1])
	}
	if len(name) >= 2 {
		return strings.ToUpper(name[:2])
	}
	return strings.ToUpper(name)
}

// GradientRule draws a horizontal ramp, used to underline headers.
func GradientRule(width int) string {
	if width <= 0 {
		return ""
	}
	return Gradient(strings.Repeat("─", width), GradientFrom, GradientTo)
}

// ---------------------------------------------------------------------------
// Meters
// ---------------------------------------------------------------------------

// MeterPlain renders an unstyled proportional bar. Use this inside rows that
// the caller will colour as a whole.
func MeterPlain(fraction float64, width int) string {
	if width <= 0 {
		return ""
	}
	fraction = clampUnit(fraction)
	exact := fraction * float64(width)
	full := int(exact)
	if full > width {
		full = width
	}
	bar := strings.Repeat(barFull, full)

	// Render the leftover eighth so short bars still show movement.
	if full < width {
		partials := []rune(barPartial)
		step := int(math.Round((exact - float64(full)) * float64(len(partials)-1)))
		if step > 0 && step < len(partials) {
			bar += string(partials[step])
			full++
		}
	}
	if pad := width - full; pad > 0 {
		bar += strings.Repeat(barEmpty, pad)
	}
	return bar
}

// Meter is MeterPlain with the filled portion coloured.
func Meter(fraction float64, width int, color lipgloss.TerminalColor) string {
	if width <= 0 {
		return ""
	}
	plain := MeterPlain(fraction, width)
	runes := []rune(plain)
	split := 0
	for split < len(runes) && string(runes[split]) != barEmpty {
		split++
	}
	filled := lipgloss.NewStyle().Foreground(color).Render(string(runes[:split]))
	rest := lipgloss.NewStyle().Foreground(Border).Render(string(runes[split:]))
	return filled + rest
}

// ---------------------------------------------------------------------------
// Stacked bars
// ---------------------------------------------------------------------------

// Segment is one slice of a StackedBar.
type Segment struct {
	Label string
	Value int
	Color lipgloss.TerminalColor
}

// StackedBar shows composition across a fixed width. Any segment with a
// non-zero value keeps at least one cell so small-but-present categories do
// not silently vanish.
func StackedBar(segments []Segment, width int) string {
	if width <= 0 {
		return ""
	}
	total := 0
	nonZero := 0
	for _, segment := range segments {
		if segment.Value > 0 {
			total += segment.Value
			nonZero++
		}
	}
	if total == 0 {
		return lipgloss.NewStyle().Foreground(Border).Render(strings.Repeat(barEmpty, width))
	}
	if nonZero > width {
		// Not enough cells to represent every category honestly.
		return lipgloss.NewStyle().Foreground(Border).Render(strings.Repeat(barFull, width))
	}

	widths := make([]int, len(segments))
	assigned := 0
	for index, segment := range segments {
		if segment.Value <= 0 {
			continue
		}
		cells := int(math.Floor(float64(segment.Value) / float64(total) * float64(width)))
		if cells < 1 {
			cells = 1
		}
		widths[index] = cells
		assigned += cells
	}
	// Hand leftover cells to the largest segments, and reclaim overflow from
	// them too, so the bar is always exactly `width` cells.
	for assigned != width {
		target, best := -1, -1
		for index, segment := range segments {
			if segment.Value <= 0 {
				continue
			}
			if assigned < width {
				if segment.Value > best {
					best, target = segment.Value, index
				}
				continue
			}
			if widths[index] > 1 && segment.Value > best {
				best, target = segment.Value, index
			}
		}
		if target < 0 {
			break
		}
		if assigned < width {
			widths[target]++
			assigned++
		} else {
			widths[target]--
			assigned--
		}
	}

	var out strings.Builder
	for index, segment := range segments {
		if widths[index] <= 0 {
			continue
		}
		out.WriteString(lipgloss.NewStyle().
			Foreground(segment.Color).
			Render(strings.Repeat(barFull, widths[index])))
	}
	return out.String()
}

// StackedLegend labels a StackedBar as "▪ 3 overdue  ▪ 1 due soon".
func StackedLegend(segments []Segment) string {
	parts := make([]string, 0, len(segments))
	for _, segment := range segments {
		if segment.Value <= 0 {
			continue
		}
		parts = append(parts,
			lipgloss.NewStyle().Foreground(segment.Color).Render("▪")+
				" "+ValueStyle.Render(fmt.Sprintf("%d", segment.Value))+
				" "+SubtitleStyle.Render(segment.Label),
		)
	}
	if len(parts) == 0 {
		return SubtitleStyle.Render("nothing to show yet")
	}
	return strings.Join(parts, SubtitleStyle.Render("   "))
}

// ---------------------------------------------------------------------------
// Sparkline
// ---------------------------------------------------------------------------

// SparklinePlain renders values as an unstyled block histogram, one cell per
// value, trimmed to width from the right (most recent wins).
func SparklinePlain(values []int, width int) string {
	if width <= 0 || len(values) == 0 {
		return ""
	}
	if len(values) > width {
		values = values[len(values)-width:]
	}
	peak := 0
	for _, value := range values {
		if value > peak {
			peak = value
		}
	}
	ramp := []rune(sparkRamp)
	var out strings.Builder
	for _, value := range values {
		switch {
		case value <= 0:
			out.WriteRune(' ')
		case peak <= 0:
			out.WriteRune(ramp[0])
		default:
			level := int(math.Round(float64(value) / float64(peak) * float64(len(ramp)-1)))
			out.WriteRune(ramp[level])
		}
	}
	return out.String()
}

// SparklineBaseline colours a histogram across the theme ramp, with empty
// buckets drawn as a dim baseline, which keeps the window's full span
// visible when activity is sparse.
func SparklineBaseline(values []int, width int) string {
	plain := SparklinePlain(values, width)
	if plain == "" {
		return ""
	}
	runes := []rune(plain)
	var out strings.Builder
	span := math.Max(1, float64(len(runes)-1))
	for index, glyph := range runes {
		if glyph == ' ' {
			out.WriteString(lipgloss.NewStyle().Foreground(Border).Render("·"))
			continue
		}
		color := BlendHex(GradientFrom, GradientTo, float64(index)/span)
		out.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(string(glyph)))
	}
	return out.String()
}

// ---------------------------------------------------------------------------
// Badges and chips
// ---------------------------------------------------------------------------

// Pill renders a filled badge — use for the selected/active state.
func Pill(label string, fg, bg lipgloss.TerminalColor) string {
	return lipgloss.NewStyle().Foreground(fg).Background(bg).Bold(true).Render(" " + label + " ")
}

// Tag renders an outlined badge, quieter than Pill — use for secondary
// metadata that isn't a selection state.
func Tag(label string, color lipgloss.TerminalColor) string {
	return lipgloss.NewStyle().Foreground(color).Bold(true).Render("[" + label + "]")
}

// ---------------------------------------------------------------------------
// Structure: rules, panels, tabs, columns
// ---------------------------------------------------------------------------

// Rule draws a labelled divider that fills the given width.
func Rule(label string, width int) string {
	if width <= 0 {
		return ""
	}
	if label == "" {
		return lipgloss.NewStyle().Foreground(Border).Render(strings.Repeat("─", width))
	}
	head := LabelStyle.Bold(true).Render(strings.ToUpper(label))
	remaining := width - lipgloss.Width(head) - 1
	if remaining < 1 {
		return FitLine(head, width)
	}
	return head + " " + lipgloss.NewStyle().Foreground(Border).Render(strings.Repeat("─", remaining))
}

// Panel draws a rounded box whose title sits inside the top border. width
// and height are the outer dimensions; a height of 0 fits the body.
func Panel(title string, accent lipgloss.TerminalColor, body string, width, height int) string {
	if width < 6 {
		return ClampBlock(body, max(0, width), max(0, height))
	}
	border := lipgloss.NewStyle().Foreground(accent)
	innerWidth := width - 4

	top := border.Render("╭─")
	used := 2
	if title != "" && innerWidth > lipgloss.Width(title)+4 {
		label := lipgloss.NewStyle().Foreground(accent).Bold(true).Render(strings.ToUpper(title))
		top += " " + label + " "
		used += lipgloss.Width(label) + 2
	}
	top += border.Render(strings.Repeat("─", max(0, width-used-1))) + border.Render("╮")

	bodyHeight := height - 2
	if height <= 0 {
		bodyHeight = lipgloss.Height(body)
	}
	lines := strings.Split(ClampBlock(body, innerWidth, max(1, bodyHeight)), "\n")
	rows := make([]string, 0, len(lines)+2)
	rows = append(rows, top)
	for _, line := range lines {
		rows = append(rows, border.Render("│")+" "+line+" "+border.Render("│"))
	}
	rows = append(rows, border.Render("╰"+strings.Repeat("─", width-2)+"╯"))
	return strings.Join(rows, "\n")
}

// TabBar renders tabs as labels with an accent underline beneath the active
// one. Returns a two-line block; callers that cannot spare the second row
// should use TabBarCompact.
func TabBar(labels []string, active, width int) string {
	if len(labels) == 0 || width <= 0 {
		return ""
	}
	const gap = 3
	var row strings.Builder
	underline := make([]string, 0, len(labels))
	for index, label := range labels {
		if index > 0 {
			row.WriteString(strings.Repeat(" ", gap))
			underline = append(underline, strings.Repeat(" ", gap))
		}
		span := lipgloss.Width(label)
		if index == active {
			row.WriteString(lipgloss.NewStyle().Foreground(Primary).Bold(true).Render(label))
			underline = append(underline, lipgloss.NewStyle().Foreground(Primary).Render(strings.Repeat("━", span)))
			continue
		}
		row.WriteString(SubtitleStyle.Render(label))
		underline = append(underline, lipgloss.NewStyle().Foreground(Border).Render(strings.Repeat("─", span)))
	}
	return FitLine(row.String(), width) + "\n" + FitLine(strings.Join(underline, ""), width)
}

// TabBarCompact renders a single-line tab strip for short terminals. Each
// label carries its own padding and the parts butt together, so this is
// never wider than the two-line TabBar for the same labels.
func TabBarCompact(labels []string, active, width int) string {
	parts := make([]string, 0, len(labels))
	for index, label := range labels {
		if index == active {
			parts = append(parts, Pill(label, PrimaryFg, Primary))
			continue
		}
		parts = append(parts, SubtitleStyle.Render(" "+label+" "))
	}
	return FitLine(strings.Join(parts, ""), width)
}

// HelpStyles themes the bubbles help component, which otherwise renders in a
// fixed grey that ignores the active palette.
func HelpStyles() help.Styles {
	keyStyle := lipgloss.NewStyle().Foreground(Primary).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(Muted)
	sepStyle := lipgloss.NewStyle().Foreground(Border)
	return help.Styles{
		ShortKey:       keyStyle,
		ShortDesc:      descStyle,
		ShortSeparator: sepStyle,
		Ellipsis:       sepStyle,
		FullKey:        keyStyle,
		FullDesc:       descStyle,
		FullSeparator:  sepStyle,
	}
}

// JoinColumns places two blocks side by side at exact widths and height, so a
// split view can never drift by a cell.
func JoinColumns(left, right string, leftWidth, gap, rightWidth, height int) string {
	leftLines := strings.Split(ClampBlock(left, leftWidth, height), "\n")
	rightLines := strings.Split(ClampBlock(right, rightWidth, height), "\n")
	spacer := strings.Repeat(" ", max(0, gap))
	rows := make([]string, 0, height)
	for index := 0; index < height; index++ {
		rows = append(rows, leftLines[index]+spacer+rightLines[index])
	}
	return strings.Join(rows, "\n")
}

// ---------------------------------------------------------------------------
// Exact-rectangle helpers
// ---------------------------------------------------------------------------

// FitLine truncates or pads a line to exactly width cells.
func FitLine(value string, width int) string {
	if width <= 0 {
		return ""
	}
	value = ansi.Truncate(value, width, "")
	if pad := width - ansi.StringWidth(value); pad > 0 {
		value += strings.Repeat(" ", pad)
	}
	return value
}

// ClampBlock forces text into an exact width x height rectangle.
func ClampBlock(value string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := strings.Split(value, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	out := make([]string, 0, height)
	for _, line := range lines {
		out = append(out, FitLine(line, width))
	}
	for len(out) < height {
		out = append(out, strings.Repeat(" ", width))
	}
	return strings.Join(out, "\n")
}
