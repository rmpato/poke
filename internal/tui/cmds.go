package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rmpato/poke/internal/capture"
	"github.com/rmpato/poke/internal/clipboard"
	"github.com/rmpato/poke/internal/history"
	"github.com/rmpato/poke/internal/runner"
	"github.com/rmpato/poke/internal/store"
)

// Messages. Every side effect in pogo is a command that reports back as one of
// these, so Update stays a pure state transition and can be tested by feeding
// it messages directly.

type entriesMsg struct {
	entries []*history.Entry
	skipped int
	err     error
}

type bodiesMsg struct {
	id       string
	request  []byte
	response []byte
	err      error
}

type replayMsg struct {
	id      string
	summary string
	err     error
}

type mutationMsg struct {
	status string
	err    error
}

type copiedMsg struct {
	what string
	err  error
}

type editorDoneMsg struct {
	text string
	err  error
}

type statusClearMsg int

type nowMsg time.Time

// loadEntries reads history off disk.
func loadEntries(st *store.Store) tea.Cmd {
	return func() tea.Msg {
		res, err := st.Load()
		return entriesMsg{entries: res.Entries, skipped: res.Skipped, err: err}
	}
}

// loadBodies fetches an entry's payloads, which live outside the index so that
// opening pogo stays fast no matter how much data has flowed through it.
func loadBodies(st *store.Store, e *history.Entry) tea.Cmd {
	id := e.ID
	var reqRef, resRef *history.BodyRef
	if e.Request.Body != nil {
		reqRef = e.Request.Body
	}
	if e.Response != nil && e.Response.Body != nil {
		resRef = e.Response.Body
	}
	return func() tea.Msg {
		msg := bodiesMsg{id: id}
		var err error
		if msg.request, err = st.Body(reqRef); err != nil {
			msg.err = err
		}
		if msg.response, err = st.Body(resRef); err != nil {
			msg.err = err
		}
		return msg
	}
}

// replay re-runs an entry through the same execution path poke uses.
func replay(rec *capture.Recorder, e *history.Entry) tea.Cmd {
	return func() tea.Msg {
		res, err := rec.Replay(context.Background(), e)
		return replayResult(res, err)
	}
}

// runEdited executes a command the user modified, recording it as a new entry
// whose parent is the request it came from.
func runEdited(rec *capture.Recorder, parent *history.Entry, args []string) tea.Cmd {
	return func() tea.Msg {
		res, err := rec.RunEdited(context.Background(), parent, args)
		return replayResult(res, err)
	}
}

// replayResult renders the one-line outcome shown after a run.
func replayResult(res *capture.Result, err error) replayMsg {
	if err != nil && (res == nil || res.Run == nil || !res.Run.Started) {
		return replayMsg{err: err}
	}
	msg := replayMsg{}
	if res.Entry != nil {
		msg.id = res.Entry.ID
	}

	var b strings.Builder
	if res.Entry != nil {
		e := res.Entry
		if s := e.Status(); s > 0 {
			b.WriteString(fmt.Sprintf("→ %d", s))
			if r := e.FinalBlock().Reason; r != "" {
				b.WriteString(" " + r)
			}
		} else {
			b.WriteString("→ " + runner.ExitMessage(e.Exit))
		}
		b.WriteString(" · " + e.Duration.String())
		if e.Response != nil && e.Response.Body != nil {
			b.WriteString(" · " + bytesHuman(e.Response.Body.Size))
		}
	} else {
		b.WriteString("ran (not recorded)")
	}
	if err != nil {
		// The request itself went through; only recording failed.
		b.WriteString(" · not saved: " + err.Error())
	}
	msg.summary = b.String()
	return msg
}

func setFavorite(st *store.Store, id string, fav bool) tea.Cmd {
	return func() tea.Msg {
		if err := st.SetFavorite(id, fav); err != nil {
			return mutationMsg{err: err}
		}
		if fav {
			return mutationMsg{status: "starred"}
		}
		return mutationMsg{status: "unstarred"}
	}
}

func deleteEntry(st *store.Store, id string) tea.Cmd {
	return func() tea.Msg {
		if err := st.Delete(id); err != nil {
			return mutationMsg{err: err}
		}
		return mutationMsg{status: "deleted"}
	}
}

func copyText(what, text string) tea.Cmd {
	return func() tea.Msg {
		if strings.TrimSpace(text) == "" {
			return copiedMsg{what: what, err: fmt.Errorf("nothing to copy")}
		}
		if _, err := clipboard.Copy(text); err != nil {
			return copiedMsg{what: what, err: err}
		}
		return copiedMsg{what: what}
	}
}

func clearStatus(token int) tea.Cmd {
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg {
		return statusClearMsg(token)
	})
}

// tickNow refreshes the clock so relative ages stay accurate in a window left
// open on a second monitor.
func tickNow() tea.Cmd {
	return tea.Tick(15*time.Second, func(t time.Time) tea.Msg { return nowMsg(t) })
}

// openEditor suspends the TUI and hands the terminal to $EDITOR, which is what
// a terminal user expects when a command line needs real editing.
func openEditor(text string) tea.Cmd {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}

	tmp, err := os.CreateTemp("", "pogo-*.sh")
	if err != nil {
		return func() tea.Msg { return editorDoneMsg{err: err} }
	}
	name := tmp.Name()
	if _, err := tmp.WriteString(text); err != nil {
		tmp.Close()
		return func() tea.Msg { return editorDoneMsg{err: err} }
	}
	tmp.Close()

	// $EDITOR may itself be a command line ("code -w", "emacsclient -nw").
	fields := strings.Fields(editor)
	cmd := exec.Command(fields[0], append(fields[1:], name)...)

	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		defer os.Remove(name)
		if err != nil {
			return editorDoneMsg{err: err}
		}
		data, err := os.ReadFile(name)
		if err != nil {
			return editorDoneMsg{err: err}
		}
		return editorDoneMsg{text: strings.TrimRight(string(data), "\n")}
	})
}
