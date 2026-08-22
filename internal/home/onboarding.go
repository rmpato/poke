package home

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/term"

	"github.com/rmpato/pogo/internal/ui"
)

// Step is one entry in the first-run "how this works" walkthrough. Body is
// the full explanation; Compact is a one-line fallback shown when the
// terminal is too narrow/short for Body — write both, don't rely on
// truncation to shorten Body for you (mid-sentence truncation reads worse
// than a purpose-written short form).
type Step struct {
	Title   string
	Body    string
	Compact string
	// Badge marks a step as newly added (e.g. "new") — optional.
	Badge string
}

// OnboardingConfig describes one onboarding screen.
type OnboardingConfig struct {
	AppName string
	// Intro is a one-line subtitle under the title, e.g. "Your workflow
	// loop" — describe the loop, not a feature list.
	Intro string
	Steps []Step
	// Art is optional decorative content (e.g. a logo/mascot rendered in
	// block glyphs) shown above the steps only when there's room. Leave
	// empty to skip this section entirely.
	Art string
	// SecondaryKey/SecondaryLabel add a second call-to-action beyond
	// "continue" (e.g. SecondaryKey: "s", SecondaryLabel: "setup"). Leave
	// both empty to only offer continue.
	SecondaryKey   string
	SecondaryLabel string
}

type onboardingModel struct {
	cfg       OnboardingConfig
	width     int
	height    int
	secondary bool
	done      bool
}

func newOnboardingModel(cfg OnboardingConfig) onboardingModel {
	return onboardingModel{cfg: cfg, width: 110, height: 34}
}

func (m onboardingModel) Init() tea.Cmd { return nil }

func (m onboardingModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		if m.cfg.SecondaryKey != "" && msg.String() == m.cfg.SecondaryKey {
			return m.finish(true)
		}
		switch msg.String() {
		case "enter", " ", "esc", "q", "ctrl+c":
			return m.finish(false)
		}
	}
	return m, nil
}

func (m onboardingModel) finish(secondary bool) (tea.Model, tea.Cmd) {
	m.secondary = secondary
	m.done = true
	return m, tea.Quit
}

func (m onboardingModel) View() string {
	width, height := m.width, m.height
	if width <= 0 {
		width = 110
	}
	if height <= 0 {
		height = 34
	}

	// ModalStyle contributes 6 columns and 4 rows from border + padding.
	modalWidth := min(96, max(30, width-8))
	modalWidth = min(modalWidth, max(20, width-2))
	modalHeight := min(32, max(12, height-6))
	modalHeight = min(modalHeight, max(6, height-2))
	contentWidth := max(14, modalWidth-6)
	contentHeight := max(2, modalHeight-4)

	header := m.renderHeader(contentWidth)
	cta := m.renderCTA(contentWidth)
	footer := m.renderFooter(contentWidth)

	// Decorative art is a first-impression flourish, not a layout
	// primitive — it only earns its space on the roomiest terminals, and
	// never at the cost of the steps collapsing to titles-only.
	art := ""
	artHeight := 0
	if m.cfg.Art != "" && contentWidth >= 40 && contentHeight >= 28 {
		art = lipgloss.NewStyle().Width(contentWidth).Align(lipgloss.Center).Render(m.cfg.Art)
		artHeight = lipgloss.Height(art) + 1
	}

	stepsHeight := max(1, contentHeight-artHeight-lipgloss.Height(header)-lipgloss.Height(cta)-lipgloss.Height(footer)-3)
	parts := make([]string, 0, 9)
	if art != "" {
		parts = append(parts, art, "")
	}
	parts = append(parts,
		header,
		"",
		m.renderSteps(contentWidth, stepsHeight),
		"",
		cta,
		"",
		footer,
	)
	content := strings.Join(parts, "\n")
	modal := ui.ModalStyle.Render(ui.ClampBlock(content, contentWidth, contentHeight))
	placed := lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
	return ui.ClampBlock(placed, width, height)
}

func (m onboardingModel) renderHeader(width int) string {
	title := ui.TitleStyle.Render("How this works")
	subtitle := ui.BrandStyle.Render(strings.ToUpper(m.cfg.AppName))
	if width < 40 {
		subtitle = ""
	}
	gap := width - lipgloss.Width(title) - lipgloss.Width(subtitle)
	line := ui.FitLine(title+strings.Repeat(" ", max(0, gap))+subtitle, width)
	intro := ui.ValueStyle.Render(ui.FitLine(m.cfg.Intro, width))
	return line + "\n" + intro
}

func (m onboardingModel) renderSteps(width, height int) string {
	titlesOnly := height < 12 || width < 48
	compact := width < 72 || height < 18
	lines := make([]string, 0, height)
	for index, step := range m.cfg.Steps {
		number := ui.TitleStyle.Render(fmt.Sprintf(" %d ", index+1))
		title := ui.ValueStyle.Render(step.Title)
		if step.Badge != "" {
			title += "  " + ui.WarningStyle.Render(step.Badge)
		}
		lines = append(lines, ui.FitLine(number+" "+title, width))
		if titlesOnly {
			continue
		}
		detail := step.Body
		if compact {
			detail = step.Compact
		}
		wrapped := ansi.Wordwrap(detail, max(12, width-4), " -/")
		for _, line := range strings.Split(wrapped, "\n") {
			lines = append(lines, ui.FitLine("    "+ui.SubtitleStyle.Render(line), width))
		}
		if index < len(m.cfg.Steps)-1 && height >= 16 {
			lines = append(lines, "")
		}
	}
	return ui.ClampBlock(strings.Join(lines, "\n"), width, height)
}

func (m onboardingModel) renderCTA(width int) string {
	primary := ui.ActiveTabStyle.Render(" enter continue ")
	row := primary
	if m.cfg.SecondaryKey != "" && width >= 42 {
		secondary := ui.InactiveTabStyle.Render(" " + m.cfg.SecondaryKey + " " + m.cfg.SecondaryLabel + " ")
		row = primary + "  " + secondary
	}
	return ui.FitLine(row, width)
}

func (m onboardingModel) renderFooter(width int) string {
	help := "enter continue"
	if m.cfg.SecondaryKey != "" {
		help += " · " + m.cfg.SecondaryKey + " " + m.cfg.SecondaryLabel
	}
	help += " · esc/q back"
	return ui.FitLine(ui.SubtitleStyle.Render(help), width)
}

// ShowOnboarding opens the first-run walkthrough. It returns true if the
// secondary action was chosen, false if the user just continued (or the
// terminal isn't interactive, in which case it returns immediately).
func ShowOnboarding(cfg OnboardingConfig) (bool, error) {
	if !term.IsTerminal(int(os.Stdout.Fd())) || !term.IsTerminal(int(os.Stdin.Fd())) {
		return false, nil
	}
	result, err := tea.NewProgram(newOnboardingModel(cfg), tea.WithAltScreen()).Run()
	if err != nil {
		return false, err
	}
	out := result.(onboardingModel)
	if !out.done {
		return false, nil
	}
	return out.secondary, nil
}
