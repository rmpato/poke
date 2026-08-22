package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/rmpato/pogo/internal/curlargs"
	"github.com/rmpato/pogo/internal/ui"
	"github.com/rmpato/pogo/internal/version"
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

// Every overlay below is the kit's Modal: one width, one border, one place on
// screen (whis SYSTEM_DESIGN.md §6.4). A dialog that is its own shape each time
// makes a program feel assembled from parts.

// renderCopyMenu draws the copy picker.
func (m *Model) renderCopyMenu(width, height int) string {
	items := m.copyItems()
	inner := ui.ModalWidth(width) - 6

	lines := make([]string, 0, len(items)+2)
	for i, it := range items {
		row := ui.StatusBar("  "+it.label, ui.Keycap(it.key)+" ", inner)
		if i == m.copyCursor {
			row = ui.SelectedRowStyle.Render(ui.StatusBar("  "+it.label, it.key+" ", inner))
		}
		lines = append(lines, row)
	}
	lines = append(lines, "", ui.SubtitleStyle.Render(
		ui.Keycap("↑↓")+" choose · "+ui.Keycap("enter")+" copy · "+ui.Keycap("esc")+" cancel"))

	return ui.Modal("Copy", strings.Join(lines, "\n"), width, height)
}

// renderConfirm draws the delete confirmation.
//
// It defaults to canceling, so a reflex Enter cannot destroy anything.
func (m *Model) renderConfirm(width, height int) string {
	e := m.entryByID(m.confirmID)
	if e == nil {
		return ""
	}
	body := strings.Join([]string{
		methodStyle(e.Request.Method).Render(e.Request.Method) + " " +
			styText.Render(truncateMiddle(m.displayURL(e), maxInt(20, ui.ModalWidth(width)-16))),
		"",
		ui.SubtitleStyle.Render("Its stored payloads go too. This cannot be undone."),
	}, "\n")

	return ui.ConfirmModal("Delete this request?", body, "Delete", "Keep", false, width, height)
}

// renderUpdateConfirm asks before replacing the binary on disk.
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

	body := strings.Join([]string{
		styText.Render(version.Version) + styFaint.Render("  →  ") + styOK.Render(m.updateVersion),
		"",
		ui.SubtitleStyle.Render("Replaces pogo in"),
		styText.Render(dir),
		"",
		ui.SubtitleStyle.Render("The download is verified against the published checksums."),
		ui.SubtitleStyle.Render("Your request history is not touched."),
	}, "\n")

	return ui.ConfirmModal("Update available", body, "Update", "Not now", true, width, height)
}

// renderEnvPicker lists the environments and marks the active one.
//
// It shows what each one defines for the API under the cursor, because
// "staging" and "staging-2" are otherwise indistinguishable at the moment you
// have to choose between them.
func (m *Model) renderEnvPicker(width, height int) string {
	names := append([]string{""}, m.envSet.Names()...)
	inner := ui.ModalWidth(width) - 6
	domain := m.domainOf(m.selected())

	lines := make([]string, 0, len(names)+3)
	for i, name := range names {
		mark := " "
		if name == m.envSet.Active {
			mark = "●"
		}
		label, detail := "(none)", "variables are left unresolved"
		if name != "" {
			label, detail = name, m.envSet.Describe(domain, name)
		}

		left := mark + " " + label
		if i == m.envCursor {
			lines = append(lines, ui.SelectedRowStyle.Render(ui.StatusBar(" "+left, detail+" ", inner)))
			continue
		}
		lines = append(lines, ui.StatusBar(
			" "+styOK.Render(mark)+" "+styText.Render(label),
			ui.SubtitleStyle.Render(detail+" "), inner))
	}

	lines = append(lines, "", ui.SubtitleStyle.Render(
		"An environment name is global; its values belong to an API."))
	if domain != "" {
		lines = append(lines, ui.SubtitleStyle.Render("Showing what each holds for "+domain+"."))
	}

	return ui.Modal("Environment", strings.Join(lines, "\n"), width, height)
}

// renderCollectionPrompt asks which collection a request belongs to.
func (m *Model) renderCollectionPrompt(width, height int) string {
	e := m.selected()
	if e == nil {
		return ""
	}

	lines := []string{
		methodStyle(e.Request.Method).Render(e.Request.Method) + " " +
			styText.Render(truncateMiddle(m.displayURL(e), maxInt(20, ui.ModalWidth(width)-16))),
		"",
		m.collectionInput.View(),
	}
	if known := m.knownCollections(); len(known) > 0 {
		lines = append(lines, "", ui.SubtitleStyle.Render("existing: "+strings.Join(known, ", ")))
	}

	return ui.Modal("Collection", strings.Join(lines, "\n"), width, height)
}

// lipglossPad pads already-styled text to a width without breaking its escapes.
func lipglossPad(s string, width int) string {
	if gap := width - lipgloss.Width(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return s
}
