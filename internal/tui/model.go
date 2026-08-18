// Package tui implements pogo, the terminal UI over poke's request history.
//
// The model owns no files and shells out to nothing: reading and writing
// history goes through internal/store, and running a request goes through
// internal/capture, which is the same path poke itself takes. Everything here
// is state and rendering.
package tui

import (
	"errors"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/rmpato/poke/internal/capture"
	"github.com/rmpato/poke/internal/config"
	"github.com/rmpato/poke/internal/history"
	"github.com/rmpato/poke/internal/selfupdate"
	"github.com/rmpato/poke/internal/store"
	"github.com/rmpato/poke/internal/version"
)

type screen int

const (
	screenList screen = iota
	screenDetail
	screenEdit
	screenDiff
	screenHelp
)

type overlay int

const (
	overlayNone overlay = iota
	overlayCopy
	overlayConfirm
	overlayUpdate
)

// row is one line of the history list. A row is either a group header or an
// entry, which keeps grouped and flat rendering on a single code path.
type row struct {
	entry  *history.Entry
	header bool
	group  string
	count  int
}

// Model is the pogo application state.
type Model struct {
	cfg config.Config
	st  *store.Store
	rec *capture.Recorder

	entries []*history.Entry
	rows    []row
	cursor  int
	top     int

	width, height int
	screen        screen
	overlay       overlay
	prevScreen    screen

	grouped   bool
	collapsed map[string]bool

	search    textinput.Model
	searching bool
	query     Query

	detail detailModel
	editor textarea.Model
	editID string

	diffA     *history.Entry
	diffVP    viewport.Model
	diffTitle string

	copyCursor int
	confirmID  string

	// updateVersion is a newer release the user has been told about. Nothing is
	// installed until they press u and confirm.
	updateVersion string
	updating      bool

	// pendingSelect moves the cursor onto an entry that does not exist yet,
	// which is how a replay lands on its own result once the reload arrives.
	pendingSelect string

	spinner spinner.Model
	busy    string

	status    string
	statusErr bool
	statusTok int
	reveal    bool
	loading   bool
	loadErr   error
	skipped   int
	now       time.Time
}

// New builds the application model.
func New(cfg config.Config, st *store.Store, rec *capture.Recorder) *Model {
	ti := textinput.New()
	ti.Prompt = "/"
	ti.Placeholder = "url, method:POST, status:4xx, host:api.example.com, is:starred"
	ti.CharLimit = 200

	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.CharLimit = 0

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = styMuted

	return &Model{
		cfg:       cfg,
		st:        st,
		rec:       rec,
		search:    ti,
		editor:    ta,
		spinner:   sp,
		collapsed: map[string]bool{},
		loading:   true,
		now:       time.Now(),
		width:     80,
		height:    24,
	}
}

// Init starts the first history load.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		loadEntries(m.st),
		m.spinner.Tick,
		tickNow(),
		checkForUpdate(m.cfg.Dir(), version.Version,
			m.cfg.Update.CheckInterval(), !m.cfg.Update.Disabled),
	)
}

// Update routes a message to the active screen.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		return m, nil

	case spinner.TickMsg:
		if m.busy == "" && !m.loading {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case nowMsg:
		m.now = time.Time(msg)
		return m, tickNow()

	case entriesMsg:
		return m, m.applyEntries(msg)

	case bodiesMsg:
		m.detail.setBodies(msg)
		m.layout()
		return m, nil

	case replayMsg:
		return m, m.applyReplay(msg)

	case diffResultMsg:
		if msg.err != nil {
			m.flashErr("could not compare: " + msg.err.Error())
			return m, clearStatus(m.statusTok)
		}
		m.diffTitle = msg.title
		m.status, m.statusErr = "", false // the "marked" hint has served its purpose
		m.diffVP.SetContent(msg.body)
		m.diffVP.GotoTop()
		m.screen = screenDiff
		m.layout()
		return m, nil

	case mutationMsg:
		m.busy = ""
		if msg.err != nil {
			m.flashErr(msg.err.Error())
			return m, nil
		}
		if msg.status != "" {
			m.flash(msg.status)
		}
		return m, loadEntries(m.st)

	case copiedMsg:
		m.busy = ""
		if msg.err != nil {
			m.flashErr("copy failed: " + msg.err.Error())
		} else {
			m.flash(msg.what + " copied")
		}
		return m, nil

	case updateAvailableMsg:
		m.updateVersion = msg.version
		return m, nil

	case updateDoneMsg:
		m.updating = false
		m.busy = ""
		switch {
		case errors.Is(msg.err, selfupdate.ErrUpToDate):
			m.updateVersion = ""
			m.flash("already up to date")
		case msg.err != nil:
			m.flashErr("update failed: " + msg.err.Error())
		default:
			m.updateVersion = ""
			m.flash(fmt.Sprintf("updated to %s — restart pogo to run the new version", msg.result.To))
		}
		return m, clearStatus(m.statusTok)

	case statusClearMsg:
		if int(msg) == m.statusTok {
			m.status, m.statusErr = "", false
		}
		return m, nil

	case editorDoneMsg:
		if msg.err != nil {
			m.flashErr(msg.err.Error())
			return m, nil
		}
		m.editor.SetValue(msg.text)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

// applyEntries installs a freshly loaded history, keeping the selection on the
// same entry when it still exists so a background reload does not move the
// cursor under the user's hands.
func (m *Model) applyEntries(msg entriesMsg) tea.Cmd {
	m.loading = false
	m.loadErr = msg.err
	if msg.err != nil {
		return nil
	}

	selected := m.selectedID()
	m.entries = msg.entries
	m.skipped = msg.skipped
	m.rebuildRows()
	if m.pendingSelect != "" {
		selected, m.pendingSelect = m.pendingSelect, ""
	}
	if selected != "" {
		m.selectID(selected)
	}
	m.layout()

	if m.screen == screenDetail {
		return m.loadDetail()
	}
	return nil
}

func (m *Model) applyReplay(msg replayMsg) tea.Cmd {
	m.busy = ""
	if msg.err != nil {
		m.flashErr(msg.err.Error())
		return nil
	}
	m.flash(msg.summary)
	m.pendingSelect = msg.id
	return loadEntries(m.st)
}

// layout recomputes child component sizes for the current terminal size.
func (m *Model) layout() {
	m.search.Width = maxInt(20, m.width-6)

	bodyH := m.contentHeight()
	m.detail.resize(m.detailWidth(), bodyH)

	m.diffVP.Width = m.width
	m.diffVP.Height = bodyH

	m.editor.SetWidth(maxInt(20, m.width-4))
	m.editor.SetHeight(maxInt(3, bodyH-8))
}

// contentHeight is the space between the header and footer chrome.
func (m *Model) contentHeight() int {
	// header (2) + footer (2)
	h := m.height - 4
	if m.searching || m.status != "" {
		h--
	}
	return maxInt(3, h)
}

// previewWidth returns the width of the list's side preview, or 0 when the
// terminal is too narrow to justify one. Below this width the list itself needs
// every column it can get.
func (m *Model) previewWidth() int {
	// Below this width the list needs every column it has, and a preview would
	// starve both panes rather than help either.
	const minSplit = 132
	if m.width < minSplit || m.screen != screenList {
		return 0
	}
	return clampInt(m.width/3, 44, 60)
}

func (m *Model) detailWidth() int {
	if w := m.previewWidth(); w > 0 && m.screen == screenList {
		return w - 2
	}
	return m.width
}

func (m *Model) listWidth() int {
	if w := m.previewWidth(); w > 0 {
		return m.width - w - 1
	}
	return m.width
}

// --- selection -------------------------------------------------------------

func (m *Model) selected() *history.Entry {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	return m.rows[m.cursor].entry
}

func (m *Model) selectedID() string {
	if e := m.selected(); e != nil {
		return e.ID
	}
	return ""
}

func (m *Model) selectID(id string) {
	for i, r := range m.rows {
		if r.entry != nil && r.entry.ID == id {
			m.cursor = i
			m.ensureVisible()
			return
		}
	}
	if m.cursor >= len(m.rows) {
		m.cursor = maxInt(0, len(m.rows)-1)
	}
}

func (m *Model) entryByID(id string) *history.Entry {
	for _, e := range m.entries {
		if e.ID == id {
			return e
		}
	}
	return nil
}

// --- status ----------------------------------------------------------------

func (m *Model) flash(text string)    { m.setStatus(text, false) }
func (m *Model) flashErr(text string) { m.setStatus(text, true) }

func (m *Model) setStatus(text string, isErr bool) {
	m.status, m.statusErr = text, isErr
	m.statusTok++
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
