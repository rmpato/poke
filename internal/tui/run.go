package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/rmpato/poke/internal/capture"
	"github.com/rmpato/poke/internal/config"
	"github.com/rmpato/poke/internal/store"
)

// Options is everything the TUI needs from the outside world. It holds the
// config *store* rather than a config value because a preference changed on a
// screen persists on the keypress that changed it — there is no "unsaved
// settings" state to get wrong (whis SYSTEM_DESIGN.md §11).
type Options struct {
	Config   *config.Store[config.Config]
	Store    *store.Store
	Recorder *capture.Recorder
}

// Run opens pogo's interactive face and blocks until the user quits.
func Run(opts Options) error {
	_, err := tea.NewProgram(New(opts), tea.WithAltScreen()).Run()
	return err
}
