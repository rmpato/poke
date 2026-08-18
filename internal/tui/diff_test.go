package tui

import (
	"strings"
	"testing"
)

func lines(s string) []string { return strings.Split(strings.TrimRight(s, "\n"), "\n") }

func TestDiffLinesIdentical(t *testing.T) {
	a := lines("one\ntwo\nthree")
	if got := diffLines(a, a); got != nil {
		t.Errorf("identical input should produce no edits, got %+v", got)
	}
}

func TestDiffLinesSingleChange(t *testing.T) {
	a := lines("one\ntwo\nthree")
	b := lines("one\nTWO\nthree")

	var added, removed, equal int
	for _, l := range diffLines(a, b) {
		switch l.op {
		case opInsert:
			added++
			if l.text != "TWO" {
				t.Errorf("inserted %q, want TWO", l.text)
			}
		case opDelete:
			removed++
			if l.text != "two" {
				t.Errorf("deleted %q, want two", l.text)
			}
		default:
			equal++
		}
	}
	if added != 1 || removed != 1 || equal != 2 {
		t.Errorf("got +%d -%d =%d, want +1 -1 =2", added, removed, equal)
	}
}

func TestDiffLinesInsertAndDelete(t *testing.T) {
	got := diffLines(lines("a\nb\nc"), lines("a\nb\nx\nc"))

	var added int
	for _, l := range got {
		if l.op == opInsert {
			added++
			if l.text != "x" {
				t.Errorf("inserted %q, want x", l.text)
			}
		}
		if l.op == opDelete {
			t.Errorf("nothing was removed, but the diff deleted %q", l.text)
		}
	}
	if added != 1 {
		t.Errorf("got %d insertions, want 1", added)
	}
}

// The edit script must actually reconstruct the target, or the diff is lying.
func TestDiffReconstructsBothSides(t *testing.T) {
	cases := [][2]string{
		{"a\nb\nc", "a\nb\nc"},
		{"a\nb\nc", "x\ny\nz"},
		{"", "a\nb"},
		{"a\nb", ""},
		{"1\n2\n3\n4\n5", "1\n3\n5\n6"},
		{"same\nsame\nold\nsame", "same\nsame\nnew\nsame"},
	}
	for _, c := range cases {
		a, b := lines(c[0]), lines(c[1])
		if c[0] == "" {
			a = nil
		}
		if c[1] == "" {
			b = nil
		}

		var gotA, gotB []string
		for _, l := range diffLines(a, b) {
			switch l.op {
			case opEqual:
				gotA = append(gotA, l.text)
				gotB = append(gotB, l.text)
			case opDelete:
				gotA = append(gotA, l.text)
			case opInsert:
				gotB = append(gotB, l.text)
			}
		}
		if d := diffLines(a, b); d == nil {
			continue // identical inputs produce no script by design
		}
		if strings.Join(gotA, "\n") != strings.Join(a, "\n") {
			t.Errorf("script does not reconstruct the left side:\ngot  %q\nwant %q", gotA, a)
		}
		if strings.Join(gotB, "\n") != strings.Join(b, "\n") {
			t.Errorf("script does not reconstruct the right side:\ngot  %q\nwant %q", gotB, b)
		}
	}
}

// Trimming common ends is what keeps two large, nearly identical responses
// cheap to compare.
func TestDiffHandlesLargeMostlyIdenticalInput(t *testing.T) {
	var a, b []string
	for i := 0; i < 20000; i++ {
		a = append(a, "line")
		b = append(b, "line")
	}
	b[10000] = "changed"

	got := diffLines(a, b)
	var edits int
	for _, l := range got {
		if l.op != opEqual {
			edits++
		}
	}
	if edits != 2 {
		t.Errorf("got %d edits, want 2 (one delete, one insert)", edits)
	}
}

func TestRenderHunksElidesUnchangedRuns(t *testing.T) {
	var a []string
	for i := 0; i < 50; i++ {
		a = append(a, "unchanged")
	}
	b := append([]string(nil), a...)
	b[25] = "different"

	out := renderHunks(diffLines(a, b))

	if !strings.Contains(out, "unchanged lines") {
		t.Errorf("long unchanged runs should be elided:\n%s", out)
	}
	if !strings.Contains(stripANSI(out), "+ different") {
		t.Errorf("the change is missing:\n%s", out)
	}
	if !strings.Contains(stripANSI(out), "- unchanged") {
		t.Errorf("the removed line is missing:\n%s", out)
	}
	// Context lines around the change must survive.
	if n := strings.Count(stripANSI(out), "  unchanged"); n < diffContext {
		t.Errorf("got %d context lines, want at least %d", n, diffContext)
	}
}

// The whole point of a JSON-aware diff: reordered keys and reformatting are not
// differences, but changed values are.
func TestJSONAwareDiffIgnoresFormattingButCatchesChanges(t *testing.T) {
	a := []byte(`{"id":42,"status":"pending","amount":120}`)
	reordered := []byte("{\n  \"amount\": 120,\n\n  \"id\":42,\n  \"status\":  \"pending\"\n}")
	changed := []byte(`{"id":42,"status":"completed","amount":120}`)

	canonA, _ := canonicalJSON(a)
	canonReordered, _ := canonicalJSON(reordered)
	canonChanged, _ := canonicalJSON(changed)

	if d := diffLines(lines(string(canonA)), lines(string(canonReordered))); d != nil {
		t.Errorf("reordering and reformatting should not register as a difference: %+v", d)
	}

	out := stripANSI(renderHunks(diffLines(lines(string(canonA)), lines(string(canonChanged)))))

	if !hasMarkedLine(out, "-", `"status": "pending"`) {
		t.Errorf("the removed value is not shown clearly:\n%s", out)
	}
	if !hasMarkedLine(out, "+", `"status": "completed"`) {
		t.Errorf("the added value is not shown clearly:\n%s", out)
	}
	for _, marker := range []string{"-", "+"} {
		if hasMarkedLine(out, marker, `"amount"`) {
			t.Errorf("an unchanged field appears as a change:\n%s", out)
		}
	}
}

// hasMarkedLine reports whether some line starts with marker and contains text.
func hasMarkedLine(out, marker, text string) bool {
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, marker) && strings.Contains(line, text) {
			return true
		}
	}
	return false
}
