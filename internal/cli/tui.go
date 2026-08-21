package cli

import (
	"github.com/rmpato/poke/internal/capture"
	"github.com/rmpato/poke/internal/store"
	"github.com/rmpato/poke/internal/tui"
)

// runTUI opens the interactive face of pogo.
//
// The store and the recorder are built here rather than inside the TUI so that
// the interactive and scripted paths open the same history with the same
// configuration — a replay from the UI is not a reconstruction of a request, it
// is the same code running the same argv (see docs/architecture.md).
func (a *app) runTUI() error {
	cfg := a.cfg()

	st, err := store.Open(cfg)
	if err != nil {
		return err
	}

	return tui.Run(tui.Options{
		Config:   a.store,
		Store:    st,
		Recorder: capture.New(cfg, st),
	})
}
