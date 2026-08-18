package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/rmpato/poke/internal/history"
)

// startEdit opens the editor on a copy of an entry's command.
//
// The editable representation is the curl command itself rather than a form of
// fields. That is not a shortcut: the command is what poke actually executes,
// so what you see is exactly what will run, and a curl line is already the
// lingua franca people paste into issues and share with colleagues. A structured
// editor would have to round-trip every curl option poke does not model, and
// would quietly drop the ones it could not.
func (m *Model) startEdit(e *history.Entry) {
	if e == nil {
		return
	}
	m.editID = e.ID
	m.screen = screenEdit

	// Editing always starts from the real command, never the masked one: a
	// redacted token would otherwise be sent to the server as "●●●●".
	m.editor.SetValue(e.Command.Multiline())
	m.editor.CursorEnd()
	m.layout()
	m.editor.Focus()
}

// renderEdit draws the edit screen.
func (m *Model) renderEdit(width, height int) string {
	parent := m.entryByID(m.editID)

	var head strings.Builder
	head.WriteString(styHeading.Render("EDIT & RUN"))
	if parent != nil {
		head.WriteString(styFaint.Render("  ·  based on "))
		head.WriteString(methodStyle(parent.Request.Method).Render(parent.Request.Method))
		head.WriteString(" ")
		head.WriteString(styMuted.Render(truncateMiddle(parent.Request.URL, maxInt(20, width-40))))
	}
	head.WriteString("\n")
	head.WriteString(styFaint.Render("The original stays untouched; running this records a new entry."))
	head.WriteString("\n\n")

	m.editor.SetWidth(maxInt(20, width-2))
	m.editor.SetHeight(maxInt(3, height-lipgloss.Height(head.String())-2))

	return head.String() + m.editor.View()
}

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
			{"is:starred", "only starred requests"},
			{"is:failed", "only failures"},
			{"t", "group by host"},
		}},
		{"ACT", []hint{
			{"r", "replay — records a new entry"},
			{"e", "edit the curl command, then run it"},
			{"ctrl+r", "run the edited command"},
			{"ctrl+e", "open $EDITOR while editing"},
			{"y", "copy menu (curl, URL, headers, bodies)"},
			{"s", "star / unstar"},
			{"x", "delete from history"},
			{"d", "mark for comparison, then diff against another"},
		}},
		{"INSPECT", []hint{
			{"tab / ⇧tab", "next / previous pane"},
			{"1 – 5", "jump to a pane"},
			{"v", "cycle body view: tree, pretty, raw"},
			{"space", "fold or unfold a JSON node"},
			{"S", "reveal masked secrets"},
		}},
	}

	col := func(g group) string {
		var b strings.Builder
		b.WriteString(styHeading.Render(g.title) + "\n")
		for _, it := range g.items {
			b.WriteString("  " + styKey.Render(pad(it.key, 20)) + styMuted.Render(it.desc) + "\n")
		}
		return b.String()
	}

	left := lipgloss.JoinVertical(lipgloss.Left, col(groups[0]), col(groups[1]))
	right := lipgloss.JoinVertical(lipgloss.Left, col(groups[2]), col(groups[3]))

	var content string
	if width >= 92 {
		content = lipgloss.JoinHorizontal(lipgloss.Top, left, "    ", right)
	} else {
		content = lipgloss.JoinVertical(lipgloss.Left, left, right)
	}

	footer := "\n" + styFaint.Render("history: "+m.st.Path())
	if m.cfg.Redact.Mode == history.ModeStore {
		footer += "\n" + styFaint.Render("redaction: secrets are stripped before being written to disk")
	} else if !m.cfg.Redact.Off {
		footer += "\n" + styFaint.Render("redaction: secrets are stored and masked on screen — see docs/security.md")
	}

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content+footer)
}
