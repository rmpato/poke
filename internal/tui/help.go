package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/rmpato/poke/internal/history"
)

// helpGroups fixes the order sections appear in. Anything the registry adds
// under a new name is appended rather than dropped.
var helpGroups = []string{"Navigate", "Request", "Find", "Organize", "View", "Edit", "App"}

// renderHelp is generated from the command registry, so a new action appears
// here the moment it exists. A reference that has to be maintained separately
// is a reference that goes stale.
func (m *Model) renderHelp(width, height int) string {
	byGroup := map[string][]command{}
	var order []string

	for _, c := range m.commands() {
		if _, seen := byGroup[c.group]; !seen {
			order = append(order, c.group)
		}
		byGroup[c.group] = append(byGroup[c.group], c)
	}

	// Known groups first, in their intended order, then any newcomers.
	var groups []string
	for _, g := range helpGroups {
		if _, ok := byGroup[g]; ok {
			groups = append(groups, g)
		}
	}
	for _, g := range order {
		if !contains(groups, g) {
			groups = append(groups, g)
		}
	}

	section := func(name string) string {
		var b strings.Builder
		b.WriteString(styHeading.Render(strings.ToUpper(name)) + "\n")
		for _, c := range byGroup[name] {
			// A blank key would read as "no way to do this"; naming the palette
			// is honest and points at where it lives.
			keys := styKey.Render(c.keys)
			if c.keys == "" {
				keys = styFaint.Render("palette")
			}
			b.WriteString("  " + lipglossPad(keys, 18) + styMuted.Render(c.title) + "\n")
		}
		return b.String()
	}

	// Two columns when there is room, one when there is not.
	var left, right []string
	for i, g := range groups {
		if i%2 == 0 {
			left = append(left, section(g))
		} else {
			right = append(right, section(g))
		}
	}

	content := lipgloss.JoinVertical(lipgloss.Left, append(left, right...)...)
	if width >= 92 {
		content = lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.JoinVertical(lipgloss.Left, left...), "    ",
			lipgloss.JoinVertical(lipgloss.Left, right...))
	}

	// The footer is kept to two lines: the search syntax, which is the one thing
	// not discoverable from a key, and where redaction currently stands, which
	// is the one thing worth knowing before pasting a command anywhere.
	var footer strings.Builder
	footer.WriteString("\n" + styFaint.Render(
		"Search:  method:POST · status:4xx · host:api.example.com · collection:auth · is:starred · is:failed"))

	switch {
	case m.cfg.Redact.Off:
		footer.WriteString("\n" + styFaint.Render("Redaction is off — secrets are shown in full."))
	case m.cfg.Redact.Mode == history.ModeStore:
		footer.WriteString("\n" + styFaint.Render("Secrets are stripped before being written to disk, so replays will not authenticate."))
	default:
		footer.WriteString("\n" + styFaint.Render("Secrets are stored and masked on screen — see docs/security.md."))
	}

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center,
		clampBlock(content+footer.String(), width))
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
