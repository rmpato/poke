package home

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestWrapIndex(t *testing.T) {
	cases := []struct{ index, count, want int }{
		{0, 5, 0},
		{4, 5, 4},
		{5, 5, 0},
		{-1, 5, 4},
		{-6, 5, 4},
		{0, 0, 0},
	}
	for _, c := range cases {
		if got := wrapIndex(c.index, c.count); got != c.want {
			t.Errorf("wrapIndex(%d, %d) = %d, want %d", c.index, c.count, got, c.want)
		}
	}
}

func TestGridVerticalIndexMovesByRow(t *testing.T) {
	// 6 items, 2 columns: rows are (0,1) (2,3) (4,5).
	cases := []struct{ index, direction, count, want int }{
		{0, 1, 6, 2},  // down from row 0 col 0 -> row 1 col 0
		{1, 1, 6, 3},  // down from row 0 col 1 -> row 1 col 1
		{5, -1, 6, 3}, // up from row 2 col 1 -> row 1 col 1
		{4, 1, 6, 0},  // down from the last row wraps to row 0, same column
		{5, 1, 6, 1},  // down from the last row wraps to row 0, same column
		{0, -1, 6, 4}, // up from the first row wraps to the last row, same column
		{1, -1, 6, 5}, // up from the first row wraps to the last row, same column
	}
	for _, c := range cases {
		if got := gridVerticalIndex(c.index, c.direction, c.count); got != c.want {
			t.Errorf("gridVerticalIndex(%d, %d, %d) = %d, want %d", c.index, c.direction, c.count, got, c.want)
		}
	}
}

func TestRecommendedIndexFallsBackToZero(t *testing.T) {
	cfg := Config{Items: []Item{{ID: "a"}, {ID: "b"}}, RecommendedID: "missing"}
	m := newModel(cfg)
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0 for an unknown RecommendedID", m.cursor)
	}

	cfg.RecommendedID = "b"
	m = newModel(cfg)
	if m.cursor != 1 {
		t.Errorf("cursor = %d, want 1 for RecommendedID %q", m.cursor, cfg.RecommendedID)
	}
}

// A row of stat cards has to sum to exactly the width it was given. CardStyle
// adds a border and padding to whatever it wraps, so the block inside is
// narrower than the card — an easy four cells per card to get wrong, and at
// three cards it pushes the last one off the screen.
func TestStatCardsFitTheirWidth(t *testing.T) {
	m := model{cfg: Config{Stats: []Stat{
		{Icon: "▤", Value: "12", Label: "requests"},
		{Icon: "◈", Value: "3", Label: "apis"},
		{Icon: "◇", Value: "staging", Label: "environment"},
	}}}

	for _, width := range []int{60, 80, 106, 110, 132, 180} {
		for i, line := range strings.Split(m.renderStats(width), "\n") {
			if got := lipgloss.Width(line); got > width {
				t.Errorf("at width %d, stat row %d is %d cells wide", width, i, got)
			}
		}
	}
}
