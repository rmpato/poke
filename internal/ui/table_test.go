package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func sum(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

func TestColumnWidthsSpanExactlyTheRow(t *testing.T) {
	const gap = 2
	layouts := [][]Column{
		{{Title: "a"}, {Title: "b"}, {Title: "c"}},                               // all flex
		{{Title: "a", Width: 10}, {Title: "b"}, {Title: "c", Width: 6}},          // mixed
		{{Title: "a", Width: 8}, {Title: "b", Width: 8}, {Title: "c", Width: 8}}, // all fixed
	}
	for index, columns := range layouts {
		for _, width := range []int{20, 47, 80, 120} {
			widths := ColumnWidths(columns, width)
			want := width - gap*(len(columns)-1)
			if got := sum(widths); got != want {
				t.Errorf("layout %d width %d: columns sum to %d, want %d (%v)",
					index, width, got, want, widths)
			}
			for _, w := range widths {
				if w < 1 {
					t.Errorf("layout %d width %d: produced a zero-width column %v",
						index, width, widths)
				}
			}
		}
	}
}

func TestColumnWidthsSurviveAnOvercrowdedRow(t *testing.T) {
	// Fixed columns that together want far more than the row can give.
	columns := []Column{{Title: "a", Width: 40}, {Title: "b", Width: 40}, {Title: "c"}}
	widths := ColumnWidths(columns, 24)
	if len(widths) != 3 {
		t.Fatalf("expected 3 widths, got %v", widths)
	}
	for _, w := range widths {
		if w < 1 {
			t.Fatalf("every column must keep at least one cell, got %v", widths)
		}
	}
}

func TestTableRowIsExactWidth(t *testing.T) {
	columns := []Column{
		{Title: "service", Width: 14},
		{Title: "status"},
		{Title: "p99", Width: 8, Align: AlignRight},
	}
	for _, width := range []int{30, 64, 100} {
		widths := ColumnWidths(columns, width)
		row := TableRow(columns, widths, []string{
			"a-very-long-service-name-that-will-not-fit", "healthy", "412ms",
		})
		want := sum(widths) + 2*(len(columns)-1)
		if got := ansi.StringWidth(row); got != want {
			t.Errorf("width %d: row is %d cells, want %d", width, got, want)
		}
	}
}

func TestTableRowPadsMissingCells(t *testing.T) {
	columns := []Column{{Title: "a", Width: 6}, {Title: "b", Width: 6}}
	widths := ColumnWidths(columns, 14)
	row := TableRow(columns, widths, []string{"only"})
	if got := ansi.StringWidth(row); got != sum(widths)+2 {
		t.Fatalf("a short row must still span the full width, got %d cells", got)
	}
}

func TestTableIsExactRectangle(t *testing.T) {
	columns := []Column{{Title: "name"}, {Title: "state", Width: 10}}
	rows := [][]string{{"alpha", "ok"}, {"beta", "degraded"}, {"gamma", "down"}}

	for _, height := range []int{3, 8, 20} {
		block := Table(columns, rows, 50, TableOptions{
			ShowHeader: true, Height: height, Selected: 1, ShowSelection: true,
		})
		lines := strings.Split(block, "\n")
		if len(lines) != height {
			t.Fatalf("height %d: got %d lines", height, len(lines))
		}
		for index, line := range lines {
			if got := ansi.StringWidth(line); got != 50 {
				t.Fatalf("height %d line %d: width %d, want 50", height, index, got)
			}
		}
	}
}

func TestTableWithNoRowsStillFillsItsBox(t *testing.T) {
	columns := []Column{{Title: "name"}}
	block := Table(columns, nil, 30, TableOptions{ShowHeader: true, Height: 6})
	lines := strings.Split(block, "\n")
	if len(lines) != 6 {
		t.Fatalf("expected 6 lines, got %d", len(lines))
	}
	for _, line := range lines {
		if ansi.StringWidth(line) != 30 {
			t.Fatalf("expected every line to be 30 cells, got %q", line)
		}
	}
}

func TestDefinitionListIsExactWidth(t *testing.T) {
	pairs := [][2]string{
		{"region", "eu-west-1"},
		{"a-much-longer-label", "some value that runs on and on and on"},
	}
	for _, width := range []int{24, 40, 90} {
		block := DefinitionList(pairs, width, 0)
		for _, line := range strings.Split(block, "\n") {
			if got := ansi.StringWidth(line); got != width {
				t.Errorf("width %d: line is %d cells (%q)", width, got, line)
			}
		}
	}
}
