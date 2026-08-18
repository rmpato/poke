package tui

import "github.com/charmbracelet/bubbles/key"

// keyMap holds every binding in the application.
//
// Bindings are declared once, with their help text attached, so the footer and
// the help screen are generated from the same source as the behavior. A
// shortcut cannot silently drift out of the documentation.
type keyMap struct {
	Up     key.Binding
	Down   key.Binding
	Top    key.Binding
	Bottom key.Binding
	PageUp key.Binding
	PageDn key.Binding

	Enter key.Binding
	Back  key.Binding
	Quit  key.Binding
	Help  key.Binding

	Replay key.Binding
	Edit   key.Binding
	Search key.Binding
	Copy   key.Binding
	Star   key.Binding
	Delete key.Binding
	Diff   key.Binding
	Group  key.Binding
	Reveal key.Binding

	NextTab key.Binding
	PrevTab key.Binding
	Body    key.Binding
	Toggle  key.Binding

	Run    key.Binding
	Editor key.Binding
}

var keys = keyMap{
	Up:     key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:   key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	Top:    key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("g", "top")),
	Bottom: key.NewBinding(key.WithKeys("end", "G"), key.WithHelp("G", "bottom")),
	PageUp: key.NewBinding(key.WithKeys("pgup", "ctrl+u"), key.WithHelp("ctrl+u", "page up")),
	PageDn: key.NewBinding(key.WithKeys("pgdown", "ctrl+d"), key.WithHelp("ctrl+d", "page down")),

	Enter: key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "inspect")),
	Back:  key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	Quit:  key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	Help:  key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),

	Replay: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "replay")),
	Edit:   key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit")),
	Search: key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
	Copy:   key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy")),
	Star:   key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "star")),
	Delete: key.NewBinding(key.WithKeys("x", "delete"), key.WithHelp("x", "delete")),
	Diff:   key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "diff")),
	Group:  key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "group by host")),
	Reveal: key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "reveal secrets")),

	NextTab: key.NewBinding(key.WithKeys("tab", "l", "right"), key.WithHelp("tab", "next pane")),
	PrevTab: key.NewBinding(key.WithKeys("shift+tab", "h", "left"), key.WithHelp("⇧tab", "prev pane")),
	Body:    key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "body view")),
	Toggle:  key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "fold")),

	Run:    key.NewBinding(key.WithKeys("ctrl+r", "ctrl+enter"), key.WithHelp("ctrl+r", "run")),
	Editor: key.NewBinding(key.WithKeys("ctrl+e"), key.WithHelp("ctrl+e", "$EDITOR")),
}

// hint is one footer entry.
type hint struct {
	key  string
	desc string
}

// footerHints returns the bindings worth advertising for the current screen.
// The footer is the discoverability mechanism, so it shows what is useful here
// and now rather than everything the program can do.
func (m *Model) footerHints() []hint {
	switch m.overlay {
	case overlayCopy:
		return []hint{{"↑↓", "choose"}, {"⏎", "copy"}, {"a–j", "pick directly"}, {"esc", "cancel"}}
	case overlayConfirm:
		return []hint{{"y", "delete"}, {"any other key", "cancel"}}
	}

	switch m.screen {
	case screenDetail:
		return []hint{
			{"tab", "pane"}, {"v", "body view"}, {"r", "replay"}, {"e", "edit"},
			{"y", "copy"}, {"d", "diff"}, {"esc", "back"}, {"?", "help"},
		}
	case screenEdit:
		return []hint{{"ctrl+r", "run"}, {"ctrl+e", "$EDITOR"}, {"esc", "cancel"}}
	case screenDiff:
		return []hint{{"↑↓", "scroll"}, {"d", "clear"}, {"esc", "back"}, {"q", "quit"}}
	case screenHelp:
		return []hint{{"esc", "back"}, {"q", "quit"}}
	default:
		if m.searching {
			return []hint{{"⏎", "apply"}, {"esc", "cancel"}}
		}
		return []hint{
			{"↑↓", "navigate"}, {"⏎", "inspect"}, {"r", "replay"}, {"e", "edit"},
			{"/", "search"}, {"y", "copy"}, {"?", "help"},
		}
	}
}
