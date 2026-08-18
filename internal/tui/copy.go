package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/rmpato/poke/internal/curlargs"
	"github.com/rmpato/poke/internal/version"
)

// copyItem is one entry in the copy menu.
type copyItem struct {
	key   string
	label string
	build func(m *Model) string
}

// copyItems returns the menu, ordered by how often each is wanted. "Copy as
// curl" is first and bound to the most reachable key because it is the reason
// this menu exists: it turns a remembered request back into something you can
// paste anywhere.
func (m *Model) copyItems() []copyItem {
	return []copyItem{
		{"c", "curl command", func(m *Model) string {
			e := m.selected()
			return curlargs.Render(e.Command.Args, false)
		}},
		{"C", "curl command, redacted", func(m *Model) string {
			e := m.selected()
			return curlargs.Render(m.cfg.Redact.MaskArgs(e.Command.Args), false)
		}},
		{"u", "URL", func(m *Model) string { return m.selected().Request.URL }},
		{"h", "request headers", func(m *Model) string {
			return headerText(m.selected().Request.Headers)
		}},
		{"b", "request body", func(m *Model) string { return string(m.detail.reqBody) }},
		{"r", "response body", func(m *Model) string { return string(m.detail.resBody) }},
		{"H", "response headers", func(m *Model) string {
			if b := m.selected().FinalBlock(); b != nil {
				return headerText(b.Headers)
			}
			return ""
		}},
		{"j", "entry as JSON", func(m *Model) string {
			data, err := json.MarshalIndent(m.selected(), "", "  ")
			if err != nil {
				return ""
			}
			return string(data)
		}},
	}
}

func headerText(hs []curlargs.Header) string {
	var b strings.Builder
	for _, h := range hs {
		b.WriteString(h.Name + ": " + h.Value + "\n")
	}
	return b.String()
}

// runCopy builds the requested text and puts it on the clipboard. Bodies may
// not be loaded yet when copying straight from the list, so they are fetched
// first rather than silently copying nothing.
func (m *Model) runCopy(item copyItem) tea.Cmd {
	e := m.selected()
	if e == nil {
		return nil
	}
	needsBody := item.key == "b" || item.key == "r"
	if needsBody && (!m.detail.loaded || m.detail.entryID != e.ID) {
		cmd := m.loadDetail()
		return tea.Sequence(cmd, func() tea.Msg {
			return copiedMsg{what: item.label, err: errBodyPending}
		})
	}
	return copyText(item.label, item.build(m))
}

// errBodyPending explains the one case where copying needs a second press.
var errBodyPending = errPending{}

type errPending struct{}

func (errPending) Error() string { return "payload still loading — press y again" }

// renderCopyMenu draws the modal.
func (m *Model) renderCopyMenu(width, height int) string {
	items := m.copyItems()

	var b strings.Builder
	b.WriteString(styHeading.Render("COPY") + "\n\n")
	for i, it := range items {
		cursor := "  "
		label := styText.Render(it.label)
		if i == m.copyCursor {
			cursor = styCursor.Render("▌ ")
			label = stySelected.Render(it.label)
		}
		b.WriteString(cursor + styKey.Render(pad(it.key, 3)) + label + "\n")
	}
	b.WriteString("\n" + styFaint.Render("⏎ copy   esc cancel"))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colRule).
		Padding(0, 2).
		Render(b.String())

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

// renderConfirm draws the delete confirmation.
func (m *Model) renderConfirm(width, height int) string {
	e := m.entryByID(m.confirmID)
	if e == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(styHeading.Render("DELETE THIS REQUEST?") + "\n\n")
	b.WriteString("  " + methodStyle(e.Request.Method).Render(e.Request.Method) + " " +
		styText.Render(truncateMiddle(m.displayURL(e), maxInt(20, width/2))) + "\n\n")
	b.WriteString("  " + styFaint.Render("Its stored payloads are removed too. This cannot be undone.") + "\n\n")
	b.WriteString("  " + styKey.Render("y") + styMuted.Render(" delete    ") +
		styKey.Render("any other key") + styMuted.Render(" cancel"))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colRed).
		Padding(0, 2).
		Render(b.String())

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

// renderUpdateConfirm asks before replacing the binaries on disk.
//
// The dialog names the exact directory it will write to, because "update?" is
// a different question depending on whether that is ~/.local/bin or
// /usr/local/bin.
func (m *Model) renderUpdateConfirm(width, height int) string {
	dir := "the directory pogo was installed in"
	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		dir = filepath.Dir(exe)
	}

	var b strings.Builder
	b.WriteString(styHeading.Render("UPDATE AVAILABLE") + "\n\n")
	b.WriteString("  " + styText.Render(version.Version) + styFaint.Render("  →  ") +
		styOK.Render(m.updateVersion) + "\n\n")
	b.WriteString("  " + styMuted.Render("Replaces poke and pogo in") + "\n")
	b.WriteString("  " + styText.Render(dir) + "\n\n")
	b.WriteString("  " + styFaint.Render("The download is verified against the published checksums.") + "\n")
	b.WriteString("  " + styFaint.Render("Your request history is not touched.") + "\n\n")
	b.WriteString("  " + styKey.Render("y") + styMuted.Render(" update    ") +
		styKey.Render("any other key") + styMuted.Render(" not now"))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colGreen).
		Padding(0, 2).
		Render(b.String())

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}
