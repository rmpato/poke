package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/rmpato/poke/internal/history"
)

// renderHelp is the full keymap, grouped by what the user is trying to do.
func (m *Model) renderHelp(width, height int) string {
	type group struct {
		title string
		items []hint
	}
	groups := []group{
		{"NAVIGATE", []hint{
			{"↑ / k", "previous request"},
			{"↓ / j", "next request"},
			{"g / G", "top / bottom"},
			{"ctrl+u / ctrl+d", "half page up / down"},
			{"⏎", "inspect (or fold a host group)"},
			{"esc", "back, or clear the search"},
			{"q / ctrl+c", "quit"},
		}},
		{"FIND", []hint{
			{"/", "search"},
			{"method:POST", "filter by method"},
			{"status:4xx", "filter by status class or exact code"},
			{"host:api.example.com", "filter by host"},
			{"collection:auth", "filter by collection"},
			{"is:starred", "only starred requests"},
			{"is:failed", "only failures"},
			{"t", "cycle grouping: none, host, collection"},
		}},
		{"ACT", []hint{
			{"r", "replay — records a new entry"},
			{"e", "edit fields, then run"},
			{"y", "copy menu (curl, URL, headers, bodies)"},
			{"s", "star / unstar"},
			{"c", "add to a collection"},
			{"x", "delete from history"},
			{"d", "mark for comparison, then diff against another"},
			{"E", "switch environment"},
			{"u", "install an available update"},
		}},
		{"INSPECT", []hint{
			{"tab / ⇧tab", "next / previous pane"},
			{"1 – 5", "overview, request, response, timing, raw"},
			{"v", "cycle body view: tree, pretty, raw"},
			{"space", "fold or unfold a JSON node"},
			{"S", "reveal masked secrets"},
		}},
		{"EDIT", []hint{
			{"↑↓", "move between fields"},
			{"⏎", "edit the focused field"},
			{"← →", "change the method"},
			{"ctrl+d", "remove a header or parameter"},
			{"ctrl+t", "switch between fields and raw curl"},
			{"ctrl+e", "hand the command to $EDITOR"},
			{"ctrl+r", "run it as a new entry"},
		}},
	}

	col := func(g group) string {
		var b strings.Builder
		b.WriteString(styHeading.Render(g.title) + "\n")
		for _, it := range g.items {
			b.WriteString("  " + styKey.Render(pad(it.key, 22)) + styMuted.Render(it.desc) + "\n")
		}
		return b.String()
	}

	left := lipgloss.JoinVertical(lipgloss.Left, col(groups[0]), col(groups[1]))
	right := lipgloss.JoinVertical(lipgloss.Left, col(groups[2]), col(groups[3]), col(groups[4]))

	var content string
	if width >= 92 {
		content = lipgloss.JoinHorizontal(lipgloss.Top, left, "    ", right)
	} else {
		content = lipgloss.JoinVertical(lipgloss.Left, left, right)
	}

	footer := "\n" + styFaint.Render("history: "+collapseHome(m.st.Path()))
	switch {
	case m.cfg.Redact.Off:
		footer += "\n" + styFaint.Render("redaction: off")
	case m.cfg.Redact.Mode == history.ModeStore:
		footer += "\n" + styFaint.Render("redaction: secrets are stripped before being written to disk")
	default:
		footer += "\n" + styFaint.Render("redaction: secrets are stored and masked on screen — see docs/security.md")
	}
	if m.envSet.Active != "" {
		footer += "\n" + styFaint.Render("environment: "+m.envSet.Active)
	}

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center,
		clampBlock(content+footer, width))
}
