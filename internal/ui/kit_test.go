package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestFitLineExactWidth(t *testing.T) {
	cases := []string{"", "short", strings.Repeat("x", 200)}
	for _, value := range cases {
		got := FitLine(value, 20)
		if width := ansi.StringWidth(got); width != 20 {
			t.Errorf("FitLine(%q, 20) has width %d, want 20", value, width)
		}
	}
}

func TestClampBlockExactRectangle(t *testing.T) {
	body := "one\ntwo\nthree\nfour\nfive"
	got := ClampBlock(body, 10, 3)
	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Fatalf("ClampBlock height = %d, want 3", len(lines))
	}
	for _, line := range lines {
		if width := ansi.StringWidth(line); width != 10 {
			t.Errorf("line %q has width %d, want 10", line, width)
		}
	}
}

func TestClampBlockPadsShortBlocks(t *testing.T) {
	got := ClampBlock("only line", 12, 4)
	lines := strings.Split(got, "\n")
	if len(lines) != 4 {
		t.Fatalf("ClampBlock height = %d, want 4 (padded)", len(lines))
	}
}

func TestShareWidthSumsExactly(t *testing.T) {
	for _, total := range []int{0, 1, 7, 100, 101} {
		for _, columns := range []int{1, 2, 3, 4} {
			widths := ShareWidth(total, columns)
			sum := 0
			for _, w := range widths {
				sum += w
			}
			if sum != total {
				t.Errorf("ShareWidth(%d, %d) sums to %d, want %d", total, columns, sum, total)
			}
		}
	}
}

func TestBlendHexClampsAndEndpoints(t *testing.T) {
	if got := BlendHex("#000000", "#FFFFFF", 0); got != "#000000" {
		t.Errorf("BlendHex at t=0 = %s, want #000000", got)
	}
	if got := BlendHex("#000000", "#FFFFFF", 1); got != "#FFFFFF" {
		t.Errorf("BlendHex at t=1 = %s, want #FFFFFF", got)
	}
	// Out-of-range t must clamp, not extrapolate past the endpoints.
	if got := BlendHex("#000000", "#FFFFFF", 5); got != "#FFFFFF" {
		t.Errorf("BlendHex at t=5 = %s, want clamped to #FFFFFF", got)
	}
}

func TestStackedBarSumsToWidth(t *testing.T) {
	segments := []Segment{
		{Label: "a", Value: 18},
		{Label: "b", Value: 3},
		{Label: "c", Value: 1},
	}
	for _, width := range []int{1, 5, 22, 60} {
		got := StackedBar(segments, width)
		if w := ansi.StringWidth(got); w != width {
			t.Errorf("StackedBar width = %d, want %d", w, width)
		}
	}
}

func TestMeterPlainWidth(t *testing.T) {
	for _, fraction := range []float64{-1, 0, 0.33, 0.5, 0.99, 1, 2} {
		got := MeterPlain(fraction, 24)
		if w := ansi.StringWidth(got); w != 24 {
			t.Errorf("MeterPlain(%v, 24) width = %d, want 24", fraction, w)
		}
	}
}
