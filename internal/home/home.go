// Package home implements a reusable "launcher" shell: a full-screen menu
// of workspaces the rest of the app dispatches on. It knows nothing about
// what any workspace actually does — it only renders items, tracks a
// cursor, and reports back which Item.ID was chosen.
//
// Typical use from your app's root command:
//
//	for {
//	    id, err := home.Choose(home.Config{
//	        AppName: "myapp",
//	        Tagline: "does the thing",
//	        Items: []home.Item{
//	            {ID: "dashboard", Icon: "◆", Title: "Dashboard", Description: "..."},
//	            {ID: "settings", Icon: "◇", Title: "Settings", Description: "..."},
//	        },
//	        RecommendedID: "dashboard",
//	    })
//	    if err != nil { return err }
//	    switch id {
//	    case "dashboard": runDashboard()
//	    case "settings": runSettings()
//	    case "": return nil // exit
//	    }
//	}
package home

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/term"

	"github.com/rmpato/poke/internal/ui"
)

// Item is one launchable workspace on the home menu.
type Item struct {
	ID          string
	Icon        string
	Title       string
	Description string
}

// Stat is an optional at-a-glance figure shown above the menu (e.g. "12
// open issues", "3 environments"). Purely decorative — supply none to skip
// this section entirely.
type Stat struct {
	Icon  string
	Value string
	Label string
}

// Config describes one home-shell invocation. Nothing here is persisted by
// this package — load/save whatever backs RecommendedID, Stats, etc.
// yourself and pass in a fresh Config each time you call Choose.
type Config struct {
	AppName       string
	Tagline       string
	Items         []Item
	RecommendedID string
	Stats         []Stat
	StatusMessage string
	// HelpLines is appended verbatim (one string per line) to the help
	// modal, below the key reference — use it for a CLI subcommand
	// reference so someone living in the TUI still discovers the
	// scriptable equivalents.
	HelpLines []string
}

type model struct {
	cfg      Config
	cursor   int
	width    int
	height   int
	helpOpen bool
	selected string
	done     bool
}

func newModel(cfg Config) model {
	m := model{cfg: cfg, width: 110, height: 34}
	m.cursor = m.recommendedIndex()
	return m
}

func (m model) recommendedIndex() int {
	for index, item := range m.cfg.Items {
		if item.ID == m.cfg.RecommendedID {
			return index
		}
	}
	return 0
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		if m.helpOpen {
			switch msg.String() {
			case "q", "esc", "?", "enter", "ctrl+c":
				m.helpOpen = false
			}
			return m, nil
		}

		count := len(m.cfg.Items)
		switch msg.String() {
		case "q", "ctrl+c":
			m.selected = ""
			m.done = true
			return m, tea.Quit
		case "up", "k":
			if m.usesGridNavigation() {
				m.cursor = gridVerticalIndex(m.cursor, -1, count)
			} else if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.usesGridNavigation() {
				m.cursor = gridVerticalIndex(m.cursor, 1, count)
			} else if m.cursor < count-1 {
				m.cursor++
			}
		case "left", "h", "shift+tab":
			m.cursor = wrapIndex(m.cursor-1, count)
		case "right", "l", "tab":
			m.cursor = wrapIndex(m.cursor+1, count)
		case "home", "g":
			m.cursor = 0
		case "end", "G":
			m.cursor = count - 1
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			index := int(msg.String()[0] - '1')
			if index >= 0 && index < count {
				m.cursor = index
			}
		case "?":
			m.helpOpen = true
		case "enter", " ":
			if m.cursor >= 0 && m.cursor < count {
				m.selected = m.cfg.Items[m.cursor].ID
				m.done = true
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m model) usesGridNavigation() bool {
	width, height := m.terminalSize()
	return width >= 76 && height >= 17
}

func wrapIndex(index, count int) int {
	if count <= 0 {
		return 0
	}
	index %= count
	if index < 0 {
		index += count
	}
	return index
}

// gridVerticalIndex moves by a full row (2 columns) instead of by 1, so
// up/down in a 2-column grid lands on the item directly above/below rather
// than the next item in list order.
func gridVerticalIndex(index, direction, count int) int {
	if count <= 0 {
		return 0
	}
	column := index % 2
	next := index + direction*2
	if next >= 0 && next < count {
		return next
	}
	if direction < 0 {
		next = count - 1
		if next%2 != column {
			next--
		}
		if next >= 0 {
			return next
		}
		return index
	}
	if column < count {
		return column
	}
	return 0
}

func (m model) View() string {
	width, height := m.terminalSize()
	if m.helpOpen {
		return m.renderHelp(width, height)
	}

	innerWidth := max(24, width-4)
	innerHeight := max(8, height-2)
	header := m.renderHeader(innerWidth)
	stats := ""
	if len(m.cfg.Stats) > 0 && innerHeight >= 20 {
		stats = m.renderStats(innerWidth)
	}
	footer := m.renderFooter(innerWidth)

	blocks := []string{header}
	used := lipgloss.Height(header) + lipgloss.Height(footer)
	if stats != "" {
		blocks = append(blocks, stats)
		used += lipgloss.Height(stats)
	}
	// One blank line separates every block, including menu and footer.
	used += len(blocks) + 1
	listHeight := max(1, innerHeight-used)
	blocks = append(blocks, m.renderMenu(innerWidth, listHeight), footer)
	body := strings.Join(blocks, "\n\n")
	return ui.PanelFrame(ui.ClampBlock(body, innerWidth, innerHeight), width, height)
}

func (m model) terminalSize() (int, int) {
	width, height := m.width, m.height
	if width <= 0 {
		width = 110
	}
	if height <= 0 {
		height = 34
	}
	return width, height
}

func (m model) renderHeader(width int) string {
	left := ui.Wordmark(m.cfg.AppName, width)
	right := ui.SubtitleStyle.Render(m.cfg.Tagline)
	if width < 48 {
		right = ""
	}
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	header := ui.FitLine(left+strings.Repeat(" ", max(0, gap))+right, width)
	if width < 40 {
		return header
	}
	return header + "\n" + ui.FitLine(ui.GradientRule(width), width)
}

func (m model) renderStats(width int) string {
	renderCard := func(cardWidth int, stat Stat) string {
		body := ui.BrandStyle.Render(stat.Icon+" ") +
			ui.ValueStyle.Render(ansi.Truncate(stat.Value, max(6, cardWidth-3), "…")) +
			"\n" + ui.SubtitleStyle.Render(strings.ToUpper(stat.Label))
		// CardStyle adds a border and a column of padding either side, so the
		// block inside has to be four cells narrower than the share this card
		// was given — otherwise a row of three cards overruns its container by
		// twelve, which is exactly how far off the home shell used to be.
		return ui.CardStyle.Copy().
			BorderForeground(ui.Primary).
			Render(ui.ClampBlock(body, max(6, cardWidth-4), 2))
	}
	stats := m.cfg.Stats
	columns := len(stats)
	if columns > 4 {
		columns = 4
		stats = stats[:4]
	}
	widths := ui.ShareWidth(width-(columns-1), columns)
	cards := make([]string, 0, columns)
	for index, stat := range stats {
		cards = append(cards, renderCard(widths[index], stat))
	}
	joined := make([]string, 0, columns*2-1)
	for index, card := range cards {
		if index > 0 {
			joined = append(joined, " ")
		}
		joined = append(joined, card)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, joined...)
}

func (m model) renderMenu(width, height int) string {
	if width >= 76 && height >= 17 {
		return m.renderWorkspaceGrid(width, height)
	}
	return m.renderWorkspaceList(width, height)
}

func (m model) renderWorkspaceGrid(width, height int) string {
	headerLeft := ui.LabelStyle.Bold(true).Render("  CHOOSE A WORKSPACE")
	headerRight := ui.SubtitleStyle.Render("arrows navigate · tab next · enter launch")
	gap := max(1, width-lipgloss.Width(headerLeft)-lipgloss.Width(headerRight))
	lines := []string{ui.FitLine(headerLeft+strings.Repeat(" ", gap)+headerRight, width)}

	items := m.cfg.Items
	columns := ui.ShareWidth(width-1, 2)
	for row := 0; row < (len(items)+1)/2; row++ {
		leftIndex := row * 2
		rightIndex := leftIndex + 1
		left := m.renderWorkspaceCard(items[leftIndex], leftIndex, columns[0])
		var right string
		if rightIndex < len(items) {
			right = m.renderWorkspaceCard(items[rightIndex], rightIndex, columns[1])
		} else {
			right = lipgloss.NewStyle().
				Width(columns[1]).
				Height(lipgloss.Height(left)).
				Render("")
		}
		lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right))
	}
	if m.cfg.StatusMessage != "" {
		lines = append(lines, ui.SubtitleStyle.Render(ui.FitLine("  "+m.cfg.StatusMessage, width)))
	}
	return ui.ClampBlock(strings.Join(lines, "\n"), width, height)
}

func (m model) renderWorkspaceCard(item Item, index, width int) string {
	selected := index == m.cursor
	recommended := item.ID == m.cfg.RecommendedID
	contentWidth := max(12, width-4)
	textWidth := max(10, contentWidth-2)

	key := ui.Keycap(fmt.Sprintf("%d", index+1))
	title := ui.ValueStyle.Render(item.Title)
	if selected {
		key = ui.Pill(fmt.Sprintf("%d", index+1), ui.PrimaryFg, ui.Primary)
		title = ui.BrandStyle.Render(item.Title)
	}
	badge := ""
	switch {
	case selected:
		badge = lipgloss.NewStyle().Foreground(ui.Primary).Bold(true).Render("SELECTED")
	case recommended:
		badge = ui.SuccessStyle.Render("RECOMMENDED")
	}
	titleWidth := max(8, textWidth-lipgloss.Width(key)-lipgloss.Width(item.Icon)-lipgloss.Width(badge)-6)
	top := key + "  " + ui.BrandStyle.Render(item.Icon) + "  " +
		ansi.Truncate(title, titleWidth, "…")
	if badge != "" {
		top += strings.Repeat(" ", max(1, textWidth-lipgloss.Width(top)-lipgloss.Width(badge))) + badge
	}
	top = ui.FitLine(top, textWidth)

	launch := ""
	if selected {
		launch = ui.BrandStyle.Render("ENTER →")
	}
	recommendedTag := ""
	if recommended {
		recommendedTag = ui.SuccessStyle.Render("RECOMMENDED") + "  "
	}
	descWidth := max(8, textWidth-lipgloss.Width(launch)-lipgloss.Width(recommendedTag)-2)
	bottom := recommendedTag + ui.SubtitleStyle.Render(ansi.Truncate(item.Description, descWidth, "…"))
	if launch != "" {
		bottom += strings.Repeat(" ", max(1, textWidth-lipgloss.Width(bottom)-lipgloss.Width(launch))) + launch
	}
	bottom = ui.FitLine(bottom, textWidth)

	border := lipgloss.RoundedBorder()
	var borderColor lipgloss.TerminalColor = ui.Border
	if selected {
		border = lipgloss.ThickBorder()
		borderColor = ui.Primary
	} else if recommended {
		borderColor = ui.Success
	}
	return lipgloss.NewStyle().
		Width(contentWidth).
		Border(border).
		BorderForeground(borderColor).
		Padding(0, 1).
		Render(top + "\n" + bottom)
}

func (m model) renderWorkspaceList(width, height int) string {
	lines := make([]string, 0, height)
	lines = append(lines,
		ui.LabelStyle.Bold(true).Render("  CHOOSE A WORKSPACE")+"  "+
			ui.SubtitleStyle.Render("↑/↓ navigate · enter launch"),
	)
	titleCol := 43
	if width < 96 {
		titleCol = 39
	}

	for index, item := range m.cfg.Items {
		number := fmt.Sprintf("%d", index+1)
		selected := index == m.cursor
		isRecommended := item.ID == m.cfg.RecommendedID

		titlePlain := number + "  " + item.Icon + "  " + item.Title
		if isRecommended {
			titlePlain += "  recommended"
		}

		var left string
		switch {
		case selected:
			left = titlePlain
		case isRecommended:
			left = ui.BrandStyle.Render(number+"  "+item.Icon) + "  " + ui.ValueStyle.Render(item.Title) +
				"  " + ui.SuccessStyle.Render("recommended")
		default:
			left = ui.BrandStyle.Render(number+"  "+item.Icon) + "  " + ui.ValueStyle.Render(item.Title)
		}

		row := left
		if width >= 70 && item.Description != "" {
			available := width - 2 - titleCol
			if available > 14 {
				pad := max(2, titleCol-lipgloss.Width(left))
				rendered := item.Description
				if !selected {
					rendered = ui.SubtitleStyle.Render(ansi.Truncate(item.Description, available, "…"))
				} else {
					rendered = ansi.Truncate(item.Description, available, "…")
				}
				row = left + strings.Repeat(" ", pad) + rendered
			}
		}

		row = ui.FitLine(row, width-2)
		if selected {
			lines = append(lines, ui.SelectedRowStyle.Render("▶ "+row))
		} else {
			lines = append(lines, "  "+row)
		}
	}
	if m.cfg.StatusMessage != "" {
		lines = append(lines, "", ui.SubtitleStyle.Render(ui.FitLine("  "+m.cfg.StatusMessage, width-2)))
	}
	return ui.ClampBlock(strings.Join(lines, "\n"), width, height)
}

func (m model) renderFooter(width int) string {
	hint := "↑/↓ move · tab next · 1-9 jump · enter open · q exit"
	if m.usesGridNavigation() {
		hint = "←/→ cards · ↑/↓ rows · tab next · 1-9 jump · enter launch · q exit"
	}
	left := ui.SubtitleStyle.Render(hint)
	right := ui.HelpHint()
	if lipgloss.Width(left)+lipgloss.Width(right)+2 > width {
		return ui.FitLine(right+"  "+ui.SubtitleStyle.Render("↑/↓ · enter · q"), width)
	}
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	return ui.FitLine(left+strings.Repeat(" ", max(1, gap))+right, width)
}

func (m model) renderHelp(width, height int) string {
	lines := []string{
		ui.TitleStyle.Render(m.cfg.AppName + " help"),
		"",
		ui.BrandStyle.Render("Keys"),
		"  ↑/↓ · j/k     Move selection",
		"  ←/→ · h/l     Move horizontally (grid) / cycle (list)",
		"  tab / S-tab   Next / previous item",
		"  1-9           Jump to item N",
		"  enter/space   Launch selected workspace",
		"  ?             This help",
		"  q / ctrl+c    Quit",
	}
	if len(m.cfg.HelpLines) > 0 {
		lines = append(lines, "", ui.BrandStyle.Render("Commands"))
		lines = append(lines, m.cfg.HelpLines...)
	}
	lines = append(lines, "",
		ui.SubtitleStyle.Render(ui.Keycap("enter")+"/"+ui.Keycap("esc")+"/"+ui.Keycap("?")+" close"))

	content := strings.Join(lines, "\n")
	modalWidth := min(max(42, width-16), 88)
	modal := ui.ModalStyle.Render(ui.ClampBlock(content, max(20, modalWidth-6), max(10, height-10)))
	return ui.ClampBlock(lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal), width, height)
}

// Choose opens the home shell and returns the selected Item.ID, or "" if
// the user quit without selecting anything. It requires an interactive
// terminal — check term.IsTerminal yourself before calling this if you
// need a non-interactive fallback (e.g. printing --help instead).
func Choose(cfg Config) (string, error) {
	if !term.IsTerminal(int(os.Stdout.Fd())) || !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", fmt.Errorf("home shell requires an interactive terminal")
	}
	result, err := tea.NewProgram(newModel(cfg), tea.WithAltScreen()).Run()
	if err != nil {
		return "", err
	}
	out := result.(model)
	if !out.done {
		return "", nil
	}
	return out.selected, nil
}
