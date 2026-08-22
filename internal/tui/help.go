package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/rmpato/pogo/internal/history"
	"github.com/rmpato/pogo/internal/ui"
)

// helpGroups fixes the order sections appear in. Anything the registry adds
// under a new name is appended rather than dropped.
var helpGroups = []string{"Navigate", "Request", "Find", "Organize", "View", "Edit", "App"}

// helpSections turns the command registry into the kit's help sections, so a
// new action appears in the reference the moment it exists. A reference that
// has to be maintained separately is a reference that goes stale.
func (m *Model) helpSections() []ui.HelpSection {
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

	sections := make([]ui.HelpSection, 0, len(groups))
	for _, g := range groups {
		rows := make([][2]string, 0, len(byGroup[g]))
		for _, c := range byGroup[g] {
			keys := c.keys
			if keys == "" {
				// A blank key would read as "no way to do this"; naming the
				// palette is honest and points at where it lives.
				keys = "palette"
			}
			rows = append(rows, [2]string{keys, c.title})
		}
		sections = append(sections, ui.HelpSection{Title: strings.ToUpper(g), Rows: rows})
	}
	return sections
}

// renderHelp draws the `?` reference.
//
// It ends with the two things no key can teach: the search syntax, and where
// redaction currently stands — which is what someone needs to know before
// pasting a command out of pogo into anywhere else.
func (m *Model) renderHelp(width, height int) string {
	sections := m.helpSections()
	sections = append(sections, ui.HelpSection{
		Title: "SEARCH",
		Rows: [][2]string{
			{"api:acme.com", "one API, however many hosts it has"},
			{"env:staging", "one environment of it"},
			{"method:POST", "by method"},
			{"status:4xx", "by status class, or status:404"},
			{"host:api.acme.com", "one exact host"},
			{"collection:auth", "by collection"},
			{"is:starred", "starred only"},
			{"is:failed", "failures only"},
		},
	})

	var note string
	switch {
	case m.cfg.Redact.Off:
		note = "Redaction is off — secrets are shown and stored in full."
	case m.cfg.Redact.Mode == history.ModeStore:
		note = "Secrets are stripped before being written, so replays will not authenticate."
	default:
		note = "Secrets are stored and masked on screen — see docs/security.md."
	}
	sections = append(sections, ui.HelpSection{Rows: [][2]string{{"", note}}})

	// Not the kit's HelpModal: pogo's reference is forty rows long, and a
	// centred box would clip it. The same sections, laid into whatever columns
	// the terminal has room for, so nothing is hidden behind a scroll.
	columns := 1
	switch {
	case width >= 150:
		columns = 3
	case width >= 96:
		columns = 2
	}

	block := renderHelpColumns(sections, width, columns)
	lines := strings.Split(block, "\n")

	// Whatever still does not fit scrolls, rather than being silently cut: a
	// reference whose last section is invisible is a reference that lies.
	m.helpMax = maxInt(0, len(lines)-height)
	m.helpScroll = clampInt(m.helpScroll, 0, m.helpMax)
	if m.helpScroll > 0 {
		lines = lines[m.helpScroll:]
	}
	block = ui.ClampBlock(strings.Join(lines, "\n"), width, height)

	if m.helpMax > 0 {
		bar := ui.Scrollbar(m.helpScroll, height, len(lines)+m.helpScroll, height)
		block = ui.JoinColumns(block, bar, width-1, 0, 1, height)
	}
	return block
}

// renderHelpColumns lays sections into columns of roughly equal height, so the
// reference fills the screen rather than running off the bottom of one column
// while the next stands empty.
func renderHelpColumns(sections []ui.HelpSection, width, columns int) string {
	widths := ui.ShareWidth(width-2*(columns-1), columns)

	blocks := make([]string, len(sections))
	heights := make([]int, len(sections))
	total := 0
	for i, section := range sections {
		blocks[i] = renderHelpSection(section, widths[0])
		heights[i] = lipgloss.Height(blocks[i]) + 1
		total += heights[i]
	}

	target := total / columns
	rendered := make([]string, columns)
	col, used := 0, 0
	for i, block := range blocks {
		if col < columns-1 && used > 0 && used+heights[i]/2 > target {
			col++
			used = 0
		}
		rendered[col] += block + "\n\n"
		used += heights[i]
	}

	out := rendered[0]
	tallest := 0
	for _, block := range rendered {
		tallest = maxInt(tallest, lipgloss.Height(block))
	}
	for i := 1; i < columns; i++ {
		out = ui.JoinColumns(out, rendered[i], widths[i-1], 2, widths[i], tallest)
	}
	return out
}

func renderHelpSection(section ui.HelpSection, width int) string {
	keyWidth := 0
	for _, row := range section.Rows {
		keyWidth = maxInt(keyWidth, lipgloss.Width(row[0]))
	}
	keyWidth = minInt(keyWidth, maxInt(6, width/2))

	var b strings.Builder
	if section.Title != "" {
		b.WriteString(ui.Rule(section.Title, width) + "\n")
	}
	for _, row := range section.Rows {
		key := ui.ValueStyle.Render(lipglossPad(row[0], keyWidth))
		b.WriteString(ui.FitLine("  "+key+"  "+ui.SubtitleStyle.Render(row[1]), width) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
