package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rmpato/poke/internal/config"
	"github.com/rmpato/poke/internal/ui"
)

// Settings is deliberately small: pogo works with no configuration at all, and
// this screen exists for the handful of decisions that are genuinely personal.
//
// Every change here writes to disk on the keypress that made it (whis
// SYSTEM_DESIGN.md §11). There is no "save" key, because there is no unsaved
// state to get wrong — and no way to leave the screen believing something was
// saved when it was not.

type setting struct {
	label  string
	value  func(*Model) string
	detail string
	// cycle advances the setting. A nil cycle makes the row a read-only fact,
	// which is still worth showing: half of what people want from a settings
	// screen is to find out where their files are.
	cycle func(*Model) tea.Cmd
}

func (m *Model) settings() []setting {
	return []setting{
		{
			label:  "Theme",
			value:  func(*Model) string { return ui.CurrentTheme() },
			detail: "the palette everything is drawn from",
			cycle:  (*Model).cycleTheme,
		},
		{
			label: "Secrets",
			value: func(m *Model) string {
				value := string(m.cfg.Redact.Mode)
				if m.cfg.Redact.Off {
					value = "stored in full"
				}
				return value + m.overriddenBy("POGO_REDACT",
					m.stored().Redact.Mode != m.cfg.Redact.Mode ||
						m.stored().Redact.Off != m.cfg.Redact.Off)
			},
			detail: "display: masked on screen · store: stripped before writing",
			cycle:  (*Model).cycleRedaction,
		},
		{
			label: "Release checks",
			value: func(m *Model) string {
				value := "every " + m.cfg.Update.CheckInterval().String()
				if m.cfg.Update.Disabled {
					value = "off"
				}
				return value + m.overriddenBy("POGO_NO_UPDATE_CHECK",
					m.stored().Update.Disabled != m.cfg.Update.Disabled)
			},
			detail: "the only thing pogo does over the network unasked",
			cycle:  (*Model).toggleUpdateChecks,
		},
		{
			label:  "History",
			value:  func(m *Model) string { return collapseHome(m.cfg.Dir()) },
			detail: "requests, headers and payloads — see docs/security.md",
		},
		{
			label:  "Config",
			value:  func(m *Model) string { return collapseHome(m.cfgStore.Path()) },
			detail: "written when you change something here",
		},
		{
			label:  "Environments",
			value:  func(*Model) string { return collapseHome(config.EnvFile()) },
			detail: "your variables, and the credentials in them",
		},
	}
}

// stored is what the config file says, as opposed to what this session is
// running under.
func (m *Model) stored() config.Config { return m.cfgStore.Current() }

// overriddenBy names the environment variable winning over the file, when one
// is. Without it, a screen showing "store" while the file says "display" would
// be telling the truth about this session and lying about what changing it
// does.
func (m *Model) overriddenBy(name string, differs bool) string {
	if !differs {
		return ""
	}
	return "  " + ui.SubtitleStyle.Render("("+name+")")
}

func (m *Model) renderSettings(width, height int) string {
	items := m.settings()

	var b strings.Builder
	b.WriteString(ui.Rule("Settings", width) + "\n")

	for i, s := range items {
		value := s.value(m)
		row := ui.StatusBar("  "+ui.ValueStyle.Render(s.label), styText.Render(value)+" ", width)
		if i == m.settingsCursor {
			row = ui.SelectedRowStyle.Render(ui.StatusBar("  "+s.label, value+" ", width))
		}
		b.WriteString(row + "\n")
		b.WriteString(ui.SubtitleStyle.Render(ui.FitLine("     "+s.detail, width)) + "\n")
	}

	// The theme is the one setting you cannot judge from its name, so the whole
	// palette is on screen underneath it.
	b.WriteString("\n" + ui.Rule("Palette", width) + "\n")
	swatchW := clampInt(width/3, 14, 26)
	swatches := []string{
		ui.Swatch("primary", ui.Primary, swatchW),
		ui.Swatch("success", ui.Success, swatchW),
		ui.Swatch("warning", ui.Warning, swatchW),
		ui.Swatch("danger", ui.Danger, swatchW),
		ui.Swatch("accent", ui.Alt, swatchW),
		ui.Swatch("muted", ui.Muted, swatchW),
	}
	for i := 0; i < len(swatches); i += 3 {
		end := minInt(i+3, len(swatches))
		b.WriteString(strings.Join(swatches[i:end], "  ") + "\n")
	}

	return ui.ClampBlock(b.String(), width, height)
}

func (m *Model) handleSettingsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := m.settings()
	switch msg.String() {
	case "up", "k":
		m.settingsCursor = clampInt(m.settingsCursor-1, 0, len(items)-1)
		return m, nil
	case "down", "j":
		m.settingsCursor = clampInt(m.settingsCursor+1, 0, len(items)-1)
		return m, nil
	case "enter", " ", "right", "l":
		if s := items[m.settingsCursor]; s.cycle != nil {
			return m, s.cycle(m)
		}
		return m, nil
	case "esc", "q":
		m.screen = screenList
		m.layout()
		return m, nil
	}
	return m, nil
}

// cycleTheme repaints and persists in one step.
func (m *Model) cycleTheme() tea.Cmd {
	themes := ui.Themes()
	next := themes[0]
	for i, name := range themes {
		if name == ui.CurrentTheme() {
			next = themes[(i+1)%len(themes)]
			break
		}
	}

	ui.ApplyTheme(next)
	refreshStyles()

	if err := m.cfgStore.Update(func(c *config.Config) { c.Theme = next }); err != nil {
		// The theme is already applied; only the remembering failed. Say so,
		// rather than silently reverting what the user can plainly see.
		m.flashErr("theme applied but not saved: " + err.Error())
		return clearStatus(m.statusTok)
	}
	m.cfg = m.cfgStore.Current().WithEnv()
	return nil
}

func (m *Model) cycleRedaction() tea.Cmd {
	return m.saveSetting(func(c *config.Config) {
		switch {
		case c.Redact.Off:
			c.Redact.Off, c.Redact.Mode = false, "display"
		case c.Redact.Mode == "display":
			c.Redact.Mode = "store"
		default:
			c.Redact.Off = true
		}
	}, "secrets")
}

func (m *Model) toggleUpdateChecks() tea.Cmd {
	return m.saveSetting(func(c *config.Config) {
		c.Update.Disabled = !c.Update.Disabled
	}, "release checks")
}

// saveSetting applies a change and reports the one thing worth reporting: that
// it did not stick.
func (m *Model) saveSetting(mutate func(*config.Config), what string) tea.Cmd {
	if err := m.cfgStore.Update(mutate); err != nil {
		m.flashErr("could not save " + what + ": " + err.Error())
		return clearStatus(m.statusTok)
	}
	m.cfg = m.cfgStore.Current().WithEnv()
	return nil
}

func (m *Model) doSettings() tea.Cmd {
	m.screen = screenSettings
	m.settingsCursor = 0
	m.layout()
	return nil
}
