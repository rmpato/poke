package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rmpato/pogo/internal/history"
	"github.com/rmpato/pogo/internal/store"
)

// toggleDiff marks an entry for comparison, or compares against the mark.
//
// Two presses of one key is the whole interaction: mark here, compare there. A
// dedicated selection mode would be more discoverable and much slower to use.
func (m *Model) toggleDiff(e *history.Entry) tea.Cmd {
	if m.diffA == nil {
		m.diffA = e
		m.flash("marked for comparison — press d on another request")
		return clearStatus(m.statusTok)
	}
	if m.diffA.ID == e.ID {
		m.diffA = nil
		m.flash("comparison cleared")
		return clearStatus(m.statusTok)
	}

	a, b := m.diffA, e
	m.diffA = nil
	return diffEntries(m.st, a, b)
}

// diffResultMsg carries a rendered comparison back to the UI.
type diffResultMsg struct {
	title string
	body  string
	err   error
}

// diffEntries loads both payloads and renders the comparison off the UI thread.
func diffEntries(st *store.Store, a, b *history.Entry) tea.Cmd {
	return func() tea.Msg {
		var refA, refB *history.BodyRef
		if a.Response != nil {
			refA = a.Response.Body
		}
		if b.Response != nil {
			refB = b.Response.Body
		}
		bodyA, err := st.Body(refA)
		if err != nil {
			return diffResultMsg{err: err}
		}
		bodyB, err := st.Body(refB)
		if err != nil {
			return diffResultMsg{err: err}
		}
		return diffResultMsg{
			title: fmt.Sprintf("%s  ↔  %s", shortLabel(a), shortLabel(b)),
			body:  renderDiff(a, b, bodyA, bodyB),
		}
	}
}

func shortLabel(e *history.Entry) string {
	return fmt.Sprintf("%s %s (%s)", e.Request.Method, history.PathOf(e.Request.URL),
		statusText(e.Status(), e.Exit))
}

// renderDiff produces the comparison view for two responses.
func renderDiff(a, b *history.Entry, bodyA, bodyB []byte) string {
	var out strings.Builder

	out.WriteString(section("SUMMARY"))
	out.WriteString(diffRow("status", statusText(a.Status(), a.Exit), statusText(b.Status(), b.Exit)))
	out.WriteString(diffRow("duration", a.Duration.String(), b.Duration.String()))
	out.WriteString(diffRow("size", bodySize(a), bodySize(b)))
	out.WriteString(diffRow("when", a.CreatedAt.Local().Format("15:04:05"), b.CreatedAt.Local().Format("15:04:05")))
	if a.Request.URL != b.Request.URL {
		out.WriteString(diffRow("url", a.Request.URL, b.Request.URL))
	}
	out.WriteString("\n")

	// Response headers, diffed the same way. They are where the answer usually
	// is when two apparently identical requests behave differently: a changed
	// content type, a missing cache header, a different upstream.
	out.WriteString(section("RESPONSE HEADERS"))
	if headerLines := diffLines(responseHeaderLines(a), responseHeaderLines(b)); len(headerLines) == 0 {
		out.WriteString(styFaint.Render("  identical") + "\n\n")
	} else {
		out.WriteString(renderHunks(headerLines) + "\n")
	}

	// Canonicalizing JSON first means the diff shows changed data rather than
	// reordered keys and reformatted whitespace.
	textA, jsonA := canonicalJSON(bodyA)
	textB, jsonB := canonicalJSON(bodyB)

	heading := "RESPONSE BODY"
	if jsonA && jsonB {
		heading += styFaint.Render("  (JSON-aware: keys sorted, formatting normalized)")
	}
	out.WriteString(section(heading))

	if len(bodyA) == 0 && len(bodyB) == 0 {
		out.WriteString(styFaint.Render("  neither response has a stored body") + "\n")
		return out.String()
	}

	linesA := splitLines(string(textA))
	linesB := splitLines(string(textB))
	hunks := diffLines(linesA, linesB)
	if len(hunks) == 0 {
		out.WriteString(styOK.Render("  responses are identical") + "\n")
		return out.String()
	}
	out.WriteString(renderHunks(hunks))
	return out.String()
}

// headerText renders an entry's final response headers as sorted lines.
//
// Sorting means a server that emits headers in a different order each time does
// not show up as a difference; a changed value still does.
func responseHeaderLines(e *history.Entry) []string {
	b := e.FinalBlock()
	if b == nil {
		return nil
	}
	lines := make([]string, 0, len(b.Headers))
	for _, h := range b.Headers {
		lines = append(lines, h.Name+": "+h.Value)
	}
	sort.Strings(lines)
	return lines
}

func bodySize(e *history.Entry) string {
	if e.Response == nil || e.Response.Body == nil {
		return "—"
	}
	return bytesHuman(e.Response.Body.Size)
}

func diffRow(label, a, b string) string {
	style := styText
	marker := "  "
	if a != b {
		style = styText
		marker = styYellowDot
	}
	return fmt.Sprintf("%s%s %s %s %s\n", marker, styKey.Render(pad(label, 10)),
		style.Render(pad(truncate(a, 30), 30)), styFaint.Render("→"), style.Render(truncate(b, 30)))
}

func splitLines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// --- diff algorithm --------------------------------------------------------

type diffOp uint8

const (
	opEqual diffOp = iota
	opDelete
	opInsert
)

type diffLine struct {
	op   diffOp
	text string
}

// diffLines computes a line diff using Myers' algorithm, which finds a minimal
// edit script in O(ND) time -- fast enough for response bodies without the
// quadratic memory a full DP table would need.
//
// Common prefixes and suffixes are stripped first, so two large responses that
// differ in one field cost almost nothing.
func diffLines(a, b []string) []diffLine {
	// Trim the matching ends.
	start := 0
	for start < len(a) && start < len(b) && a[start] == b[start] {
		start++
	}
	endA, endB := len(a), len(b)
	for endA > start && endB > start && a[endA-1] == b[endB-1] {
		endA--
		endB--
	}

	prefix := a[:start]
	suffix := a[endA:]
	midA, midB := a[start:endA], b[start:endB]

	if len(midA) == 0 && len(midB) == 0 {
		return nil // identical
	}

	var out []diffLine
	for _, l := range prefix {
		out = append(out, diffLine{opEqual, l})
	}
	out = append(out, myers(midA, midB)...)
	for _, l := range suffix {
		out = append(out, diffLine{opEqual, l})
	}
	return out
}

// maxDiffEdits bounds the search. Beyond it the two documents have so little in
// common that a line-by-line edit script would be noise, and the diff degrades
// to "everything was replaced", which is both honest and instant.
const maxDiffEdits = 4000

func myers(a, b []string) []diffLine {
	n, mLen := len(a), len(b)
	maxD := n + mLen
	if maxD > maxDiffEdits {
		out := make([]diffLine, 0, n+mLen)
		for _, l := range a {
			out = append(out, diffLine{opDelete, l})
		}
		for _, l := range b {
			out = append(out, diffLine{opInsert, l})
		}
		return out
	}

	offset := maxD
	v := make([]int, 2*maxD+1)
	trace := make([][]int, 0, maxD+1)

	for d := 0; d <= maxD; d++ {
		snapshot := make([]int, len(v))
		copy(snapshot, v)
		trace = append(trace, snapshot)

		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[offset+k-1] < v[offset+k+1]) {
				x = v[offset+k+1]
			} else {
				x = v[offset+k-1] + 1
			}
			y := x - k
			for x < n && y < mLen && a[x] == b[y] {
				x++
				y++
			}
			v[offset+k] = x

			if x >= n && y >= mLen {
				return backtrack(a, b, trace, offset)
			}
		}
	}
	return nil
}

// backtrack walks the recorded search states backwards to recover the edits.
func backtrack(a, b []string, trace [][]int, offset int) []diffLine {
	var rev []diffLine
	x, y := len(a), len(b)

	for d := len(trace) - 1; d >= 0 && (x > 0 || y > 0); d-- {
		v := trace[d]
		k := x - y

		var prevK int
		if k == -d || (k != d && v[offset+k-1] < v[offset+k+1]) {
			prevK = k + 1
		} else {
			prevK = k - 1
		}
		prevX := v[offset+prevK]
		prevY := prevX - prevK

		for x > prevX && y > prevY {
			x--
			y--
			rev = append(rev, diffLine{opEqual, a[x]})
		}
		if d == 0 {
			break
		}
		if x > prevX {
			x--
			rev = append(rev, diffLine{opDelete, a[x]})
		} else {
			y--
			rev = append(rev, diffLine{opInsert, b[y]})
		}
	}

	// Reverse into forward order.
	out := make([]diffLine, len(rev))
	for i, l := range rev {
		out[len(rev)-1-i] = l
	}
	return out
}

// --- diff rendering --------------------------------------------------------

// diffContext is how many unchanged lines surround a change. Three is what
// every other diff tool shows, and familiarity is worth more here than novelty.
const diffContext = 3

// renderHunks prints changed regions with context, eliding long unchanged runs.
func renderHunks(lines []diffLine) string {
	keep := make([]bool, len(lines))
	for i, l := range lines {
		if l.op == opEqual {
			continue
		}
		for j := maxInt(0, i-diffContext); j <= minInt(len(lines)-1, i+diffContext); j++ {
			keep[j] = true
		}
	}

	var (
		b       strings.Builder
		skipped int
		added   int
		removed int
	)
	for i, l := range lines {
		if !keep[i] {
			skipped++
			continue
		}
		if skipped > 0 {
			b.WriteString(styFaint.Render(fmt.Sprintf("  ⋮ %d unchanged lines", skipped)) + "\n")
			skipped = 0
		}
		switch l.op {
		case opInsert:
			added++
			b.WriteString(styDiffAdd.Render("+ "+l.text) + "\n")
		case opDelete:
			removed++
			b.WriteString(styDiffDel.Render("- "+l.text) + "\n")
		default:
			// The newline stays outside Render: lipgloss reflows and pads
			// multi-line input, which would corrupt the alignment of a diff.
			b.WriteString(styFaint.Render("  "+l.text) + "\n")
		}
	}
	if skipped > 0 {
		b.WriteString(styFaint.Render(fmt.Sprintf("  ⋮ %d unchanged lines", skipped)) + "\n")
	}

	summary := fmt.Sprintf("\n%s %s\n",
		styDiffAdd.Render(fmt.Sprintf("+%d", added)),
		styDiffDel.Render(fmt.Sprintf("-%d", removed)))
	return b.String() + summary
}
