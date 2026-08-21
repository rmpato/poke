package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func assertRectangle(t *testing.T, label, block string, width, height int) {
	t.Helper()
	lines := strings.Split(block, "\n")
	if len(lines) != height {
		t.Fatalf("%s: %d lines, want %d", label, len(lines), height)
	}
	for index, line := range lines {
		if got := ansi.StringWidth(line); got != width {
			t.Fatalf("%s: line %d is %d cells, want %d", label, index, got, width)
		}
	}
}

func TestStatusBarIsExactWidth(t *testing.T) {
	cases := []struct{ left, right string }{
		{"short", "hint"},
		{strings.Repeat("long ", 40), "hint"},
		{"left", strings.Repeat("right ", 20)},
		{"", ""},
	}
	for _, width := range []int{10, 24, 80} {
		for _, c := range cases {
			line := StatusBar(c.left, c.right, width)
			if got := ansi.StringWidth(line); got != width {
				t.Errorf("StatusBar(%q,%q,%d) is %d cells", c.left, c.right, width, got)
			}
		}
	}
}

func TestStatusBarKeepsTheRightHalf(t *testing.T) {
	// When the two halves can't both fit, the key hint is what a stuck user
	// needs, so it must survive.
	line := StatusBar(strings.Repeat("breadcrumb ", 10), "? Help", 20)
	if !strings.Contains(line, "Help") {
		t.Fatalf("expected the right half to survive truncation, got %q", line)
	}
}

func TestBreadcrumbDropsLeadingSegments(t *testing.T) {
	trail := []string{"example", "services", "eu-west-1", "search-indexer"}
	wide := Breadcrumb(trail, 60)
	if !strings.Contains(wide, "example") {
		t.Fatal("a wide breadcrumb should show the whole trail")
	}
	narrow := Breadcrumb(trail, 22)
	if ansi.StringWidth(narrow) != 22 {
		t.Fatalf("narrow breadcrumb is %d cells, want 22", ansi.StringWidth(narrow))
	}
	if !strings.Contains(narrow, "search-indexer") {
		t.Fatalf("the current segment must survive; got %q", narrow)
	}
}

func TestEmptyStateIsExactRectangle(t *testing.T) {
	for _, size := range [][2]int{{20, 5}, {40, 9}, {70, 3}} {
		block := EmptyState("◍", "Nothing here", "press n to add one", size[0], size[1])
		assertRectangle(t, "EmptyState", block, size[0], size[1])
	}
}

func TestBannerIsExactWidth(t *testing.T) {
	for _, width := range []int{20, 40, 90} {
		block := Banner(KindDanger, "Rollout failed",
			"2 of 6 replicas never became ready and the rollout was halted.", width)
		for index, line := range strings.Split(block, "\n") {
			if got := ansi.StringWidth(line); got != width {
				t.Errorf("width %d: line %d is %d cells", width, index, got)
			}
		}
	}
}

func TestBannerWithoutBodyIsOneLine(t *testing.T) {
	block := Banner(KindWarning, "Drift detected", "", 40)
	if lines := strings.Split(block, "\n"); len(lines) != 1 {
		t.Fatalf("expected a single line, got %d", len(lines))
	}
}

func TestStepListIsExactWidthForEveryState(t *testing.T) {
	steps := []ProgressStep{
		{Title: "Done", State: StepDone, Detail: "1.4.2"},
		{Title: "Active", State: StepActive, Detail: "4/6"},
		{Title: "Pending", State: StepPending},
		{Title: "Failed", State: StepFailed},
		{Title: "Skipped", State: StepSkipped},
	}
	for _, width := range []int{16, 40, 100} {
		// Frames must be safe at any counter value, including negatives.
		for _, frame := range []int{-3, 0, 7, 1000} {
			block := StepList(steps, frame, width)
			assertRectangle(t, "StepList", block, width, len(steps))
		}
	}
}

func TestModalFillsTheTerminalRectangle(t *testing.T) {
	for _, size := range [][2]int{{60, 20}, {110, 34}, {40, 14}} {
		block := Modal("Title", "body line one\nbody line two", size[0], size[1])
		assertRectangle(t, "Modal", block, size[0], size[1])
	}
}

func TestHelpModalFillsTheTerminalRectangle(t *testing.T) {
	sections := []HelpSection{
		{Title: "Keys", Rows: [][2]string{{"tab", "Next"}, {"q", "Back"}}},
		{Title: "More", Rows: [][2]string{{"ctrl+k", "Command palette"}}},
	}
	for _, size := range [][2]int{{70, 24}, {110, 34}} {
		block := HelpModal("Help", sections, size[0], size[1])
		assertRectangle(t, "HelpModal", block, size[0], size[1])
	}
}

func TestConfirmModalFillsTheTerminalRectangle(t *testing.T) {
	for _, focused := range []bool{true, false} {
		block := ConfirmModal("Delete?", "This cannot be undone.", "Delete", "Cancel", focused, 90, 28)
		assertRectangle(t, "ConfirmModal", block, 90, 28)
	}
}

func TestKindGlyphsAreOneCell(t *testing.T) {
	for _, kind := range []Kind{KindInfo, KindSuccess, KindWarning, KindDanger} {
		if got := ansi.StringWidth(kind.Glyph()); got != 1 {
			t.Errorf("kind %d glyph is %d cells, want 1", kind, got)
		}
	}
}

func TestSwatchIsExactWidth(t *testing.T) {
	for _, width := range []int{12, 30, 60} {
		if got := ansi.StringWidth(Swatch("Primary", Primary, width)); got != width {
			t.Errorf("Swatch width %d, got %d", width, got)
		}
	}
}
