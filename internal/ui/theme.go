package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Theme names. The default is pogo's own; the two alternates come from whis
// and are kept because a tool nobody can restyle feels like someone else's.
const (
	ThemeDefault = "pogo"
	ThemeSunset  = "sunset"
	ThemeForest  = "forest"
)

// Themes lists every theme name, in cycling order.
func Themes() []string { return []string{ThemeDefault, ThemeSunset, ThemeForest} }

// pogo's palette is the wordmark's blue-to-green ramp, deepened until it holds
// against a white terminal as well as a black one.
//
// Accents are fixed brand colours; anything that has to sit against the
// user's own terminal background is adaptive so the UI stays legible on both
// light and dark terminal themes.
var (
	Primary   = lipgloss.Color("#2E86F0")
	PrimaryFg = lipgloss.Color("#FFFFFF")
	Success   = lipgloss.Color("#3FB950")
	Warning   = lipgloss.Color("#D29922")
	Danger    = lipgloss.Color("#E5534B")
	// Alt is the secondary accent, for the things that are neither status nor
	// selection: a badge, a JSON literal, an imported entry. pogo needs a sixth
	// colour that means "different", and reaching for a status colour to say it
	// would make a booking look like a failure.
	Alt = lipgloss.Color("#A371F7")

	Muted  = lipgloss.AdaptiveColor{Light: "#5A6270", Dark: "#8B909A"}
	Text   = lipgloss.AdaptiveColor{Light: "#1C2028", Dark: "#C8CCD4"}
	Border = lipgloss.AdaptiveColor{Light: "#C8CDD6", Dark: "#2A2F3A"}
	// Surface is only safe behind plain, unstyled text: a nested style's
	// reset ends the background for the rest of the line, which leaves gaps
	// of the terminal's default colour.
	Surface = lipgloss.AdaptiveColor{Light: "#EDEFF3", Dark: "#1B1F27"}

	// Gradient endpoints for wordmarks, meters, and sparklines. Kept as hex
	// strings because the ramp is interpolated per cell. Blue to green is
	// pogo's identity: it is the wordmark.
	GradientFrom = "#2E86F0"
	GradientTo   = "#3FB950"
)

var (
	TitleStyle       lipgloss.Style
	BrandStyle       lipgloss.Style
	SubtitleStyle    lipgloss.Style
	SuccessStyle     lipgloss.Style
	WarningStyle     lipgloss.Style
	ErrorStyle       lipgloss.Style
	LabelStyle       lipgloss.Style
	ValueStyle       lipgloss.Style
	BoxStyle         lipgloss.Style
	CardStyle        lipgloss.Style
	SelectedRowStyle lipgloss.Style
	ActiveTabStyle   lipgloss.Style
	InactiveTabStyle lipgloss.Style
	ModalStyle       lipgloss.Style

	currentTheme = ThemeDefault
)

func init() {
	ApplyTheme(ThemeDefault)
}

// ApplyTheme switches the shared UI accent palette. Unknown names safely use
// the default theme so older or hand-edited config files remain usable.
//
// Add a new theme by adding a case here that reassigns every token, then
// call rebuildStyles() (already done at the bottom of the switch). Keep the
// full token set assigned in every branch — a theme that only overrides
// Primary will leave stale Muted/Text/Border/Surface values from whatever
// theme was active before it.
func ApplyTheme(name string) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case ThemeSunset:
		currentTheme = ThemeSunset
		Primary = lipgloss.Color("#FB923C")
		PrimaryFg = lipgloss.Color("#1F1300")
		Success = lipgloss.Color("#22C55E")
		Warning = lipgloss.Color("#F5A524")
		Danger = lipgloss.Color("#F43F5E")
		Alt = lipgloss.Color("#C77DFF")
		Muted = lipgloss.AdaptiveColor{Light: "#8A6350", Dark: "#D8A98F"}
		Text = lipgloss.AdaptiveColor{Light: "#2A160A", Dark: "#FFF3EA"}
		Border = lipgloss.AdaptiveColor{Light: "#E7C3A6", Dark: "#4A2E1C"}
		Surface = lipgloss.AdaptiveColor{Light: "#FBE7D6", Dark: "#2B1A0F"}
		GradientFrom = "#FB923C"
		GradientTo = "#F43F5E"
		SelectedRowStyle = lipgloss.NewStyle().
			Foreground(PrimaryFg).
			Background(lipgloss.Color("#E17A2E")).
			Bold(true)
	case ThemeForest:
		currentTheme = ThemeForest
		Primary = lipgloss.Color("#22C55E")
		PrimaryFg = lipgloss.Color("#052E14")
		Success = lipgloss.Color("#4ADE80")
		Warning = lipgloss.Color("#F5A524")
		Danger = lipgloss.Color("#F43F5E")
		Alt = lipgloss.Color("#7CA9F0")
		Muted = lipgloss.AdaptiveColor{Light: "#3F6B52", Dark: "#8FD6AC"}
		Text = lipgloss.AdaptiveColor{Light: "#0B2416", Dark: "#EAFBF0"}
		Border = lipgloss.AdaptiveColor{Light: "#A9DDBD", Dark: "#1E4B31"}
		Surface = lipgloss.AdaptiveColor{Light: "#DDF3E5", Dark: "#0F2A1A"}
		GradientFrom = "#22C55E"
		GradientTo = "#3B82F6"
		SelectedRowStyle = lipgloss.NewStyle().
			Foreground(PrimaryFg).
			Background(lipgloss.Color("#1FA850")).
			Bold(true)
	default:
		currentTheme = ThemeDefault
		Primary = lipgloss.Color("#2E86F0")
		PrimaryFg = lipgloss.Color("#FFFFFF")
		Success = lipgloss.Color("#3FB950")
		Warning = lipgloss.Color("#D29922")
		Danger = lipgloss.Color("#E5534B")
		Alt = lipgloss.Color("#A371F7")
		Muted = lipgloss.AdaptiveColor{Light: "#5A6270", Dark: "#8B909A"}
		Text = lipgloss.AdaptiveColor{Light: "#1C2028", Dark: "#C8CCD4"}
		Border = lipgloss.AdaptiveColor{Light: "#C8CDD6", Dark: "#2A2F3A"}
		Surface = lipgloss.AdaptiveColor{Light: "#EDEFF3", Dark: "#1B1F27"}
		GradientFrom = "#2E86F0"
		GradientTo = "#3FB950"
		SelectedRowStyle = lipgloss.NewStyle().
			Foreground(PrimaryFg).
			Background(lipgloss.Color("#1F62B8")).
			Bold(true)
	}
	rebuildStyles()
}

func CurrentTheme() string {
	return currentTheme
}

// PrimaryRGB exposes the active accent as raw channels, for bridging into
// non-lipgloss color types (e.g. Fang's color scheme, which wants
// image/color.Color rather than a lipgloss.Color).
func PrimaryRGB() (uint8, uint8, uint8) {
	switch currentTheme {
	case ThemeSunset:
		return 0xFB, 0x92, 0x3C
	case ThemeForest:
		return 0x22, 0xC5, 0x5E
	default:
		return 0x2E, 0x86, 0xF0
	}
}

func rebuildStyles() {
	TitleStyle = lipgloss.NewStyle().Bold(true).Foreground(PrimaryFg).Background(Primary).Padding(0, 1)
	BrandStyle = lipgloss.NewStyle().Bold(true).Foreground(Primary)
	SubtitleStyle = lipgloss.NewStyle().Foreground(Muted)
	SuccessStyle = lipgloss.NewStyle().Foreground(Success).Bold(true)
	WarningStyle = lipgloss.NewStyle().Foreground(Warning).Bold(true)
	ErrorStyle = lipgloss.NewStyle().Foreground(Danger).Bold(true)
	LabelStyle = lipgloss.NewStyle().Foreground(Muted)
	ValueStyle = lipgloss.NewStyle().Foreground(Text).Bold(true)
	BoxStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(Primary).Padding(0, 1)
	CardStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(Border).Foreground(Text).Padding(0, 1)
	ActiveTabStyle = lipgloss.NewStyle().Bold(true).Foreground(PrimaryFg).Background(Primary).Padding(0, 2)
	InactiveTabStyle = lipgloss.NewStyle().Foreground(Muted).Background(Surface).Padding(0, 2)
	ModalStyle = lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(Primary).Foreground(Text).Padding(1, 2)
}

func Info(msg string) {
	fmt.Fprintln(os.Stderr, LabelStyle.Render("→"), msg)
}

func Ok(msg string) {
	fmt.Fprintln(os.Stderr, SuccessStyle.Render("✓"), msg)
}

func Warn(msg string) {
	fmt.Fprintln(os.Stderr, WarningStyle.Render("!"), msg)
}

func Fail(msg string) {
	fmt.Fprintln(os.Stderr, ErrorStyle.Render("✗"), msg)
}

func KeyValue(key, value string) string {
	return fmt.Sprintf("%s %s", LabelStyle.Render(key+":"), ValueStyle.Render(value))
}

// Keycap renders a keyboard key hint like [?] with brand emphasis.
func Keycap(key string) string {
	return BrandStyle.Render("[" + key + "]")
}

// HelpHint is a short, discoverable callout for contextual help. Every
// screen's footer should render this on its trailing edge.
func HelpHint() string {
	return Keycap("?") + " " + BrandStyle.Render("Help")
}

// PanelFrame gives every full-screen workspace the same rounded, branded
// shell while pinning output to the exact terminal rectangle. Every
// top-level View() should return through this.
func PanelFrame(body string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	panelWidth := width - 2
	if panelWidth < 10 {
		panelWidth = 10
	}
	boxed := BoxStyle.Copy().Width(panelWidth).Render(body)
	placed := lipgloss.Place(width, height, lipgloss.Center, lipgloss.Top, boxed)
	return exactRectangle(placed, width, height)
}

func exactRectangle(value string, width, height int) string {
	lines := strings.Split(value, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	out := make([]string, 0, height)
	for _, line := range lines {
		line = ansi.Truncate(line, width, "")
		if pad := width - ansi.StringWidth(line); pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		out = append(out, line)
	}
	for len(out) < height {
		out = append(out, strings.Repeat(" ", width))
	}
	return strings.Join(out, "\n")
}

// HuhTheme returns the active accent theme for Huh forms, so wizard-style
// screens (setup flows, confirmations) match the Bubble Tea screens instead
// of Huh's own default charm theme.
func HuhTheme() *huh.Theme {
	t := huh.ThemeCharm()
	t.Focused.Title = t.Focused.Title.Foreground(Primary).Bold(true)
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(Primary)
	t.Focused.SelectedPrefix = t.Focused.SelectedPrefix.Foreground(Primary)
	t.Focused.Base = t.Focused.Base.BorderForeground(Primary)
	t.Focused.Description = t.Focused.Description.Foreground(Muted)
	t.Blurred.Title = t.Blurred.Title.Foreground(Muted)

	// ThemeCharm's buttons, text cursor and error text keep Charm's own
	// palette, which is the one place a Huh form gives itself away as a
	// different product from the screen that opened it. Re-point every
	// remaining accent at the theme tokens.
	t.Focused.FocusedButton = t.Focused.FocusedButton.
		Foreground(PrimaryFg).Background(Primary).Bold(true)
	t.Focused.BlurredButton = t.Focused.BlurredButton.
		Foreground(Muted).Background(Surface)
	t.Blurred.FocusedButton = t.Focused.FocusedButton
	t.Blurred.BlurredButton = t.Focused.BlurredButton

	t.Focused.TextInput.Cursor = t.Focused.TextInput.Cursor.Foreground(Primary)
	t.Focused.TextInput.Prompt = t.Focused.TextInput.Prompt.Foreground(Primary)
	t.Focused.TextInput.Placeholder = t.Focused.TextInput.Placeholder.Foreground(Border)
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(Primary)
	t.Focused.MultiSelectSelector = t.Focused.MultiSelectSelector.Foreground(Primary)
	t.Focused.NoteTitle = t.Focused.NoteTitle.Foreground(Primary).Bold(true)
	t.Focused.ErrorIndicator = t.Focused.ErrorIndicator.Foreground(Danger)
	t.Focused.ErrorMessage = t.Focused.ErrorMessage.Foreground(Danger)
	t.Blurred.NoteTitle = t.Blurred.NoteTitle.Foreground(Muted)
	t.Help = HelpStyles()
	return t
}
