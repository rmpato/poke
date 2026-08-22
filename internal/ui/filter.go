package ui

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Fuzzy matching
// ---------------------------------------------------------------------------
//
// Everything filterable in a TUI — a command palette, a `/` filter over a
// list, a picker — wants the same behavior: type a few letters, get the
// obvious thing first. Keep one implementation so ranking feels identical
// everywhere, rather than each screen inventing its own `strings.Contains`.

// MatchResult is a scored fuzzy match: Positions holds the index of every
// rune in the target that the pattern consumed, so callers can highlight
// exactly what matched.
type MatchResult struct {
	Score     int
	Positions []int
}

// FuzzyMatch reports whether pattern appears in target as a subsequence,
// scoring matches so that better ones sort first. Matching is
// case-insensitive; an empty pattern matches everything with score 0.
//
// The scoring is deliberately simple and explainable — consecutive runs,
// word starts, and a prefix bonus — because a ranking a user can't predict
// feels broken even when it's technically better.
func FuzzyMatch(pattern, target string) (MatchResult, bool) {
	if pattern == "" {
		return MatchResult{}, true
	}

	needle := []rune(strings.ToLower(pattern))
	hay := []rune(target)
	lowerHay := []rune(strings.ToLower(target))

	positions := make([]int, 0, len(needle))
	score, at := 0, 0

	for _, want := range needle {
		found := -1
		for index := at; index < len(lowerHay); index++ {
			if lowerHay[index] == want {
				found = index
				break
			}
		}
		if found < 0 {
			return MatchResult{}, false
		}

		switch {
		case found == 0:
			score += 12 // matched the very start of the candidate
		case isWordStart(hay, found):
			score += 8 // matched the start of a word or camelCase hump
		case len(positions) > 0 && positions[len(positions)-1] == found-1:
			score += 5 // extends a consecutive run
		default:
			score++
		}
		// Everything skipped over is noise; prefer tighter matches.
		score -= (found - at) / 4

		positions = append(positions, found)
		at = found + 1
	}

	// Short candidates that used most of their runes are usually the one the
	// user meant ("ui" should beat "user-interface-builder").
	score += 10 * len(needle) / max(1, len(hay))
	return MatchResult{Score: score, Positions: positions}, true
}

func isWordStart(runes []rune, index int) bool {
	if index == 0 {
		return true
	}
	prev, cur := runes[index-1], runes[index]
	if prev == ' ' || prev == '-' || prev == '_' || prev == '.' || prev == '/' {
		return true
	}
	return unicode.IsLower(prev) && unicode.IsUpper(cur)
}

// FuzzyRank filters candidates by pattern and returns the surviving indices,
// best first. Ties keep the caller's original order, so an unfiltered list
// stays in the order the screen chose to present it.
func FuzzyRank(pattern string, candidates []string) []int {
	type scored struct {
		index, score int
	}
	hits := make([]scored, 0, len(candidates))
	for index, candidate := range candidates {
		if result, ok := FuzzyMatch(pattern, candidate); ok {
			hits = append(hits, scored{index: index, score: result.Score})
		}
	}
	sort.SliceStable(hits, func(a, b int) bool { return hits[a].score > hits[b].score })

	out := make([]int, 0, len(hits))
	for _, hit := range hits {
		out = append(out, hit.index)
	}
	return out
}

// HighlightMatch renders target with the runes that pattern matched picked
// out in the accent color. base styles everything else, so this works both
// on a normal row and inside an already-colored one.
func HighlightMatch(pattern, target string, base lipgloss.Style) string {
	result, ok := FuzzyMatch(pattern, target)
	if !ok || len(result.Positions) == 0 {
		return base.Render(target)
	}

	hit := make(map[int]bool, len(result.Positions))
	for _, position := range result.Positions {
		hit[position] = true
	}
	accent := base.Foreground(Primary).Bold(true)

	var out strings.Builder
	for index, glyph := range []rune(target) {
		if hit[index] {
			out.WriteString(accent.Render(string(glyph)))
			continue
		}
		out.WriteString(base.Render(string(glyph)))
	}
	return out.String()
}

// ---------------------------------------------------------------------------
// Scroll window maths
// ---------------------------------------------------------------------------

// Window returns the [start, end) slice of a total-length list to draw so
// that cursor stays visible in a viewport of height rows. It keeps the
// cursor away from the very edge where possible, which is what stops a list
// from feeling like it's dragging the selection along the bottom line.
func Window(cursor, height, total int) (int, int) {
	if height <= 0 || total <= 0 {
		return 0, 0
	}
	if total <= height {
		return 0, total
	}

	// One row of lookahead at each end, as long as the viewport can spare it.
	margin := 0
	if height >= 5 {
		margin = 1
	}

	start := cursor - margin
	if start < 0 {
		start = 0
	}
	if start+height > total {
		start = total - height
	}
	if cursor < start {
		start = cursor
	}
	if cursor >= start+height {
		start = cursor - height + 1
	}
	return start, start + height
}

// Scrollbar draws a vertical track of exactly height rows, with a thumb
// sized to the visible fraction. Returns an empty block (spaces) when
// everything already fits, so a list doesn't jitter as it crosses the
// threshold — the column is always there, it's just blank.
func Scrollbar(start, visible, total, height int) string {
	if height <= 0 {
		return ""
	}
	rows := make([]string, height)
	if total <= visible || total <= 0 {
		for index := range rows {
			rows[index] = " "
		}
		return strings.Join(rows, "\n")
	}

	track := lipgloss.NewStyle().Foreground(Border)
	thumb := lipgloss.NewStyle().Foreground(Primary)

	size := max(1, height*visible/total)
	top := start * height / total
	if top+size > height {
		top = height - size
	}
	for index := range rows {
		if index >= top && index < top+size {
			rows[index] = thumb.Render("┃")
			continue
		}
		rows[index] = track.Render("│")
	}
	return strings.Join(rows, "\n")
}

// ---------------------------------------------------------------------------
// Filter input
// ---------------------------------------------------------------------------

// FilterBar renders the `/` filter line from the key grammar: the query with
// a block cursor while active, and a count of what survived. Pass active as
// false to render the same row as a dimmed summary of the filter still in
// effect.
func FilterBar(query string, shown, total int, width int, active bool) string {
	if width <= 0 {
		return ""
	}
	prompt := BrandStyle.Render("/")
	body := query
	if active {
		body += "▏"
	}
	if query == "" && !active {
		body = SubtitleStyle.Render("filter")
	} else {
		body = ValueStyle.Render(body)
	}

	count := SubtitleStyle.Render(fmt.Sprintf("%d/%d", shown, total))
	left := prompt + " " + body
	gap := width - lipgloss.Width(left) - lipgloss.Width(count)
	if gap < 1 {
		return FitLine(left, width)
	}
	return FitLine(left+strings.Repeat(" ", gap)+count, width)
}
