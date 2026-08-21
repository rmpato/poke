package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestFuzzyMatchSubsequence(t *testing.T) {
	if _, ok := FuzzyMatch("dsh", "Dashboard"); !ok {
		t.Fatal("expected 'dsh' to match 'Dashboard' as a subsequence")
	}
	if _, ok := FuzzyMatch("zzz", "Dashboard"); ok {
		t.Fatal("expected 'zzz' not to match 'Dashboard'")
	}
	if _, ok := FuzzyMatch("", "anything"); !ok {
		t.Fatal("expected the empty pattern to match everything")
	}
}

func TestFuzzyMatchReportsPositions(t *testing.T) {
	result, ok := FuzzyMatch("brd", "Dashboard")
	if !ok {
		t.Fatal("expected a match")
	}
	if len(result.Positions) != 3 {
		t.Fatalf("expected one position per pattern rune, got %v", result.Positions)
	}
	for index, position := range result.Positions {
		if index > 0 && position <= result.Positions[index-1] {
			t.Fatalf("positions must strictly increase, got %v", result.Positions)
		}
		if got := strings.ToLower("Dashboard")[position]; got != "brd"[index] {
			t.Fatalf("position %d points at %q, want %q", position, got, "brd"[index])
		}
	}
}

func TestFuzzyRankPrefersPrefixAndWordStarts(t *testing.T) {
	candidates := []string{"Reopen the walkthrough", "Settings", "Set theme"}
	ranked := FuzzyRank("set", candidates)
	if len(ranked) == 0 {
		t.Fatal("expected at least one match for 'set'")
	}
	if candidates[ranked[0]] != "Settings" && candidates[ranked[0]] != "Set theme" {
		t.Fatalf("expected a prefix match first, got %q", candidates[ranked[0]])
	}
	// The scattered subsequence must rank below both direct prefix matches.
	for _, index := range ranked[:min(2, len(ranked))] {
		if candidates[index] == "Reopen the walkthrough" {
			t.Fatal("a scattered subsequence outranked a prefix match")
		}
	}
}

func TestFuzzyRankEmptyPatternKeepsOrder(t *testing.T) {
	candidates := []string{"one", "two", "three"}
	ranked := FuzzyRank("", candidates)
	if len(ranked) != len(candidates) {
		t.Fatalf("expected every candidate, got %d", len(ranked))
	}
	for index, position := range ranked {
		if index != position {
			t.Fatalf("expected original order, got %v", ranked)
		}
	}
}

func TestWindowKeepsCursorVisible(t *testing.T) {
	const height, total = 5, 40
	for cursor := 0; cursor < total; cursor++ {
		start, end := Window(cursor, height, total)
		if end-start != height {
			t.Fatalf("cursor %d: window is %d rows, want %d", cursor, end-start, height)
		}
		if start < 0 || end > total {
			t.Fatalf("cursor %d: window [%d,%d) escapes [0,%d)", cursor, start, end, total)
		}
		if cursor < start || cursor >= end {
			t.Fatalf("cursor %d fell outside window [%d,%d)", cursor, start, end)
		}
	}
}

func TestWindowShortListShowsEverything(t *testing.T) {
	start, end := Window(1, 10, 3)
	if start != 0 || end != 3 {
		t.Fatalf("expected the whole list, got [%d,%d)", start, end)
	}
}

func TestScrollbarIsExactHeight(t *testing.T) {
	for _, total := range []int{0, 3, 10, 500} {
		bar := Scrollbar(0, 10, total, 8)
		if got := len(strings.Split(bar, "\n")); got != 8 {
			t.Fatalf("total %d: scrollbar is %d rows, want 8", total, got)
		}
	}
}

func TestFilterBarIsExactWidth(t *testing.T) {
	for _, width := range []int{12, 30, 80} {
		for _, active := range []bool{true, false} {
			line := FilterBar("data", 3, 42, width, active)
			if got := ansi.StringWidth(line); got != width {
				t.Fatalf("width %d active %v: got %d cells", width, active, got)
			}
		}
	}
}
