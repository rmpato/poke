package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/rmpato/pogo/internal/apis"
	"github.com/rmpato/pogo/internal/config"
	"github.com/rmpato/pogo/internal/ui"
)

// The APIs workspace is where the grouping stops being something that happens
// to you. pogo works out which API a host belongs to and which environment it
// is, and this screen shows you every one of those conclusions with the keys to
// correct it — because a guess you cannot argue with is just a bug you have to
// live with.

// apiRow is one line of the workspace: an API, or one of its environments.
type apiRow struct {
	api    apis.API
	env    *apis.Env
	isAPI  bool
	domain string
}

// apiRows flattens the summary into selectable lines.
func (m *Model) apiRows() []apiRow {
	var rows []apiRow
	for _, api := range m.apiSummary() {
		rows = append(rows, apiRow{api: api, isAPI: true, domain: api.Domain})
		for i := range api.Envs {
			rows = append(rows, apiRow{api: api, env: &api.Envs[i], domain: api.Domain})
		}
	}
	return rows
}

func (m *Model) selectedAPIRow() (apiRow, bool) {
	rows := m.apiRows()
	if m.apiCursor < 0 || m.apiCursor >= len(rows) {
		return apiRow{}, false
	}
	return rows[m.apiCursor], true
}

func (m *Model) moveAPICursor(delta int) {
	rows := m.apiRows()
	if len(rows) == 0 {
		return
	}
	m.apiCursor = clampInt(m.apiCursor+delta, 0, len(rows)-1)
}

// renderAPIs draws the workspace.
func (m *Model) renderAPIs(width, height int) string {
	rows := m.apiRows()
	if len(rows) == 0 {
		return ui.EmptyState("◈", "No APIs yet",
			"run "+ui.Keycap("pogo curl")+" against something and it appears here",
			width, height)
	}

	// Two panes: the tree on the left, what pogo concluded about the selected
	// row on the right, so a correction is made next to the thing it corrects.
	listW := clampInt(width/2, 28, 56)
	detailW := width - listW - 2

	start, end := ui.Window(m.apiCursor, height, len(rows))
	lines := make([]string, 0, height)

	for i := start; i < end; i++ {
		r := rows[i]
		var left, right string
		if r.isAPI {
			left = "◈ " + ui.Fallback(r.api.Name, r.api.Domain)
			if r.api.Hidden {
				left += " (hidden)"
			}
			right = itoa(r.api.Count)
		} else {
			left = "   " + r.env.Name
			right = itoa(r.env.Count)
		}

		if i == m.apiCursor {
			lines = append(lines, ui.SelectedRowStyle.Render(ui.StatusBar(" "+left, right+" ", listW)))
			continue
		}
		if r.isAPI {
			lines = append(lines, ui.StatusBar(
				" "+ui.ValueStyle.Render(left), ui.SubtitleStyle.Render(right+" "), listW))
			continue
		}
		lines = append(lines, ui.StatusBar(
			" "+envStyle(r.env.Name).Render(left), ui.SubtitleStyle.Render(right+" "), listW))
	}

	return ui.JoinColumns(strings.Join(lines, "\n"), m.renderAPIDetail(detailW, height),
		listW, 2, detailW, height)
}

// envStyle colors an environment by how much it would hurt to break.
func envStyle(env string) lipgloss.Style {
	switch env {
	case apis.EnvProd:
		return lipgloss.NewStyle().Foreground(ui.Danger)
	case apis.EnvPreprod, apis.EnvStaging:
		return lipgloss.NewStyle().Foreground(ui.Warning)
	case apis.EnvLocal, apis.EnvDev:
		return lipgloss.NewStyle().Foreground(ui.Muted)
	default:
		return lipgloss.NewStyle().Foreground(ui.Text)
	}
}

// renderAPIDetail explains what pogo decided, and how it decided it.
func (m *Model) renderAPIDetail(width, height int) string {
	row, ok := m.selectedAPIRow()
	if !ok {
		return ui.ClampBlock("", width, height)
	}

	var b strings.Builder
	if row.isAPI {
		pairs := [][2]string{{"API", ui.Fallback(row.api.Name, row.api.Domain)}}
		// The domain is only worth a line of its own once it stops being the
		// name; repeating it says nothing twice.
		if row.api.Name != "" && row.api.Name != row.api.Domain {
			pairs = append(pairs, [2]string{"Domain", row.api.Domain})
		}
		pairs = append(pairs,
			[2]string{"Requests", itoa(row.api.Count)},
			[2]string{"Environments", itoa(len(row.api.Envs))})
		b.WriteString(ui.Rule("API", width) + "\n")
		b.WriteString(ui.DefinitionList(pairs, width, 14) + "\n\n")
		b.WriteString(ui.SubtitleStyle.Render(ui.Keycap("n")+" name it · "+
			ui.Keycap("x")+" hide it · "+ui.Keycap("⏎")+" show its requests") + "\n")
	} else {
		hosts := row.env.Hosts
		b.WriteString(ui.Rule("Environment", width) + "\n")
		b.WriteString(ui.DefinitionList([][2]string{
			{"Environment", row.env.Name},
			{"API", ui.Fallback(row.api.Name, row.api.Domain)},
			{"Requests", itoa(row.env.Count)},
		}, width, 14) + "\n\n")

		b.WriteString(ui.Rule("Hosts", width) + "\n")
		for _, host := range hosts {
			mark := ui.SubtitleStyle.Render("guessed")
			if _, pinned := m.cfg.APIs.EnvFor(host); pinned {
				mark = ui.Tag("pinned", ui.Primary)
			}
			b.WriteString(ui.StatusBar("  "+styText.Render(host), mark, width) + "\n")
		}
		b.WriteString("\n" + ui.SubtitleStyle.Render(ui.Keycap("p")+" pin these hosts · "+
			ui.Keycap("⏎")+" show its requests") + "\n")
	}

	// What the active environment resolves to here, since this is the screen
	// where someone asks "which base am I actually calling?".
	if env := m.envSet.Active; env != "" {
		b.WriteString("\n" + ui.Rule("Variables · "+env, width) + "\n")
		b.WriteString(ui.SubtitleStyle.Render("  "+m.envSet.Describe(row.domain, env)) + "\n")
	}

	return ui.ClampBlock(b.String(), width, height)
}

// handleAPIKey drives the workspace.
func (m *Model) handleAPIKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.moveAPICursor(-1)
		return m, nil
	case "down", "j":
		m.moveAPICursor(1)
		return m, nil
	case "enter":
		return m, m.filterToSelectedAPI()
	case "n":
		return m, m.startAPIName()
	case "p":
		return m, m.pinSelectedEnv()
	case "x":
		return m, m.toggleAPIHidden()
	case "esc", "q":
		m.screen = screenList
		m.layout()
		return m, nil
	}
	return m, nil
}

// filterToSelectedAPI leaves the workspace showing what it was pointing at.
func (m *Model) filterToSelectedAPI() tea.Cmd {
	row, ok := m.selectedAPIRow()
	if !ok {
		return nil
	}
	query := "api:" + row.domain
	if row.env != nil {
		query += " env:" + row.env.Name
	}
	m.screen = screenList
	m.layout()
	return m.applyFilter(query)
}

// pinSelectedEnv makes a guessed environment a stated one, for every host in
// it. Pinning what is already correct is the common case: it is how you tell
// pogo that it got this one right and should stop re-deciding.
func (m *Model) pinSelectedEnv() tea.Cmd {
	row, ok := m.selectedAPIRow()
	if !ok || row.env == nil {
		return nil
	}
	hosts := row.env.Hosts
	if err := m.setAPIOverride(func(c *config.Config) {
		for _, host := range hosts {
			c.APIs.SetEnv(host, row.env.Name)
		}
	}); err != nil {
		m.flashErr("could not save: " + err.Error())
		return clearStatus(m.statusTok)
	}
	m.flash(pluralize(len(hosts), "host") + " pinned to " + row.env.Name)
	return clearStatus(m.statusTok)
}

func (m *Model) toggleAPIHidden() tea.Cmd {
	row, ok := m.selectedAPIRow()
	if !ok {
		return nil
	}
	hidden := !m.cfg.APIs.IsHidden(row.domain)
	if err := m.setAPIOverride(func(c *config.Config) {
		c.APIs.SetHidden(row.domain, hidden)
	}); err != nil {
		m.flashErr("could not save: " + err.Error())
		return clearStatus(m.statusTok)
	}
	if hidden {
		m.flash(row.domain + " hidden")
	} else {
		m.flash(row.domain + " shown")
	}
	return clearStatus(m.statusTok)
}

// startAPIName opens the rename prompt, reusing the collection input: it is the
// same interaction — one line of text about the thing under the cursor.
func (m *Model) startAPIName() tea.Cmd {
	row, ok := m.selectedAPIRow()
	if !ok {
		return nil
	}
	m.overlay = overlayAPIName
	m.collectionInput.SetValue(m.cfg.APIs.Name(row.domain))
	m.collectionInput.CursorEnd()
	return m.collectionInput.Focus()
}

// doAPIs opens the workspace.
func (m *Model) doAPIs() tea.Cmd {
	m.screen = screenAPIs
	m.apiCursor = 0
	m.layout()
	return nil
}

// renderAPINamePrompt asks what an API should be called.
func (m *Model) renderAPINamePrompt(width, height int) string {
	row, ok := m.selectedAPIRow()
	if !ok {
		return ""
	}
	body := strings.Join([]string{
		ui.SubtitleStyle.Render("Shown instead of the domain, everywhere."),
		"",
		m.collectionInput.View(),
		"",
		ui.SubtitleStyle.Render("empty goes back to " + row.domain),
	}, "\n")
	return ui.Modal("Name this API", body, width, height)
}

// pinSelectedHost states the environment of the host under the cursor, taking
// pogo's guess as the starting point. It is the one-key version of the APIs
// screen, for the moment you notice a row filed in the wrong place.
func (m *Model) pinSelectedHost() tea.Cmd {
	e := m.selected()
	if e == nil {
		return nil
	}
	ref := m.apiRef(e)
	if ref.Host == "" {
		return nil
	}
	if err := m.setAPIOverride(func(c *config.Config) {
		c.APIs.SetEnv(ref.Host, ref.Env)
	}); err != nil {
		m.flashErr("could not save: " + err.Error())
		return clearStatus(m.statusTok)
	}
	m.flash(ref.Host + " is " + ref.Env + " — change it on the APIs screen (A)")
	return clearStatus(m.statusTok)
}
