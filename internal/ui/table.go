package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// ---------------------------------------------------------------------------
// Tables
// ---------------------------------------------------------------------------
//
// A table is where the exact-rectangle contract earns its keep: the moment
// one cell is a character wider than its column, every row below it looks
// broken. Column widths are resolved once, up front, and every cell is
// clamped to its column — nothing here trusts a caller to pre-size a string.

// Align controls horizontal placement of a cell within its column.
type Align int

const (
	AlignLeft Align = iota
	AlignRight
	AlignCenter
)

// Column describes one table column. A Width of 0 means "flex": leftover
// width is shared between all flex columns, so a table always spans exactly
// the width it was given.
type Column struct {
	Title string
	Width int
	Align Align
}

// TableOptions tunes one Table render. The zero value is a headerless,
// unselected, body-height table — which is what you want when a table is
// nested inside a panel that already carries the title.
type TableOptions struct {
	// Selected highlights a row by index; -1 (or the zero value with
	// ShowSelection false) highlights nothing.
	Selected int
	// ShowSelection must be set for Selected to have any effect, so the zero
	// value doesn't accidentally highlight row 0.
	ShowSelection bool
	// ShowHeader draws the column titles plus a rule beneath them.
	ShowHeader bool
	// Height, when non-zero, clamps the whole block to exactly that many
	// rows — pad with blanks, truncate what doesn't fit.
	Height int
	// Offset is the first row index to draw, for callers paging with Window.
	Offset int
}

// ColumnWidths resolves declared widths against an available total. Fixed
// columns keep their width (shrinking proportionally only if they can't all
// fit); flex columns share what's left. The result always sums to exactly
// width minus the inter-column gaps.
func ColumnWidths(columns []Column, width int) []int {
	const gap = 2

	out := make([]int, len(columns))
	if len(columns) == 0 || width <= 0 {
		return out
	}

	available := width - gap*(len(columns)-1)
	if available < len(columns) {
		// Not enough room for even one cell per column; give everyone 1 and
		// let FitLine do the truncating.
		for index := range out {
			out[index] = 1
		}
		return out
	}

	fixedTotal, flex := 0, 0
	for _, column := range columns {
		if column.Width > 0 {
			fixedTotal += column.Width
			continue
		}
		flex++
	}

	if flex == 0 {
		// All fixed: reconcile absorbs the difference so the row still spans
		// exactly the width it was given.
		for index, column := range columns {
			out[index] = max(1, column.Width)
		}
		return reconcile(out, available)
	}

	remaining := available - fixedTotal
	if remaining < flex {
		// Fixed columns have eaten the row; squeeze them proportionally and
		// leave one cell for each flex column.
		remaining = flex
		squeezed := available - flex
		for index, column := range columns {
			if column.Width > 0 {
				out[index] = max(1, column.Width*squeezed/max(1, fixedTotal))
			}
		}
	} else {
		for index, column := range columns {
			if column.Width > 0 {
				out[index] = column.Width
			}
		}
	}

	shares := ShareWidth(remaining, flex)
	at := 0
	for index, column := range columns {
		if column.Width > 0 {
			continue
		}
		out[index] = shares[at]
		at++
	}
	return reconcile(out, available)
}

// reconcile nudges widths so they sum to exactly total, taking from (or
// giving to) the widest column — the one where a cell either way is least
// noticeable.
func reconcile(widths []int, total int) []int {
	sum := 0
	for _, width := range widths {
		sum += width
	}
	for sum != total {
		target, best := -1, 0
		for index, width := range widths {
			if width > best {
				best, target = width, index
			}
		}
		if target < 0 {
			break
		}
		if sum < total {
			widths[target]++
			sum++
			continue
		}
		if widths[target] <= 1 {
			break
		}
		widths[target]--
		sum--
	}
	return widths
}

// TableRow lays out one row of already-plain cells at the resolved widths.
// It emits no colour of its own, which is what lets a caller wrap the whole
// line in a selection style without an inner reset punching a hole in it.
func TableRow(columns []Column, widths []int, cells []string) string {
	const gap = 2

	parts := make([]string, 0, len(columns))
	for index, column := range columns {
		width := 1
		if index < len(widths) {
			width = widths[index]
		}
		cell := ""
		if index < len(cells) {
			cell = cells[index]
		}
		parts = append(parts, alignCell(cell, width, column.Align))
	}
	return strings.Join(parts, strings.Repeat(" ", gap))
}

func alignCell(value string, width int, align Align) string {
	if width <= 0 {
		return ""
	}
	value = ansi.Truncate(value, width, "…")
	pad := width - ansi.StringWidth(value)
	if pad <= 0 {
		return value
	}
	switch align {
	case AlignRight:
		return strings.Repeat(" ", pad) + value
	case AlignCenter:
		left := pad / 2
		return strings.Repeat(" ", left) + value + strings.Repeat(" ", pad-left)
	default:
		return value + strings.Repeat(" ", pad)
	}
}

// Table renders columns and rows into an exact rectangle. Cells should be
// plain text: the table owns row colour, because the selected row is drawn
// as one styled span and any colour already inside it would end the
// highlight early.
func Table(columns []Column, rows [][]string, width int, opts TableOptions) string {
	if width <= 0 || len(columns) == 0 {
		return ""
	}
	widths := ColumnWidths(columns, width)

	lines := make([]string, 0, len(rows)+2)
	if opts.ShowHeader {
		titles := make([]string, len(columns))
		for index, column := range columns {
			titles[index] = strings.ToUpper(column.Title)
		}
		header := TableRow(columns, widths, titles)
		lines = append(lines,
			LabelStyle.Bold(true).Render(FitLine(header, width)),
			lipgloss.NewStyle().Foreground(Border).Render(strings.Repeat("─", width)),
		)
	}

	offset := max(0, opts.Offset)
	for index := offset; index < len(rows); index++ {
		line := FitLine(TableRow(columns, widths, rows[index]), width)
		if opts.ShowSelection && index == opts.Selected {
			lines = append(lines, SelectedRowStyle.Render(line))
			continue
		}
		lines = append(lines, lipgloss.NewStyle().Foreground(Text).Render(line))
	}

	block := strings.Join(lines, "\n")
	if opts.Height > 0 {
		return ClampBlock(block, width, opts.Height)
	}
	return block
}

// DefinitionList renders aligned label/value pairs — the detail pane that
// usually sits next to a table. labelWidth of 0 sizes to the longest label.
func DefinitionList(pairs [][2]string, width, labelWidth int) string {
	if width <= 0 || len(pairs) == 0 {
		return ""
	}
	if labelWidth <= 0 {
		for _, pair := range pairs {
			labelWidth = max(labelWidth, lipgloss.Width(pair[0]))
		}
	}
	labelWidth = min(labelWidth, max(1, width-4))

	lines := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		label := LabelStyle.Render(alignCell(pair[0], labelWidth, AlignLeft))
		value := ValueStyle.Render(ansi.Truncate(pair[1], max(1, width-labelWidth-2), "…"))
		lines = append(lines, FitLine(label+"  "+value, width))
	}
	return strings.Join(lines, "\n")
}
