package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Notifications
// ---------------------------------------------------------------------------
//
// Toast renders one line; Notifier is the thing that decides which lines
// exist and for how long. Screens that hand-roll this always get the same
// two bugs: a message that outlives the action it described, and a second
// message that silently replaces the first before anyone read it.
//
// The rules it encodes:
//
//   - Toasts stack, newest nearest the action, oldest expiring first. A new
//     notice never overwrites an unread one.
//   - Anything above KindInfo is also kept in a history, because a warning
//     that vanished after four seconds may as well not have happened.
//   - Expiry is driven by tea.Cmd, not a goroutine, so Update stays a pure
//     state transition and nothing races the model.
//
// Wiring is three lines in a screen:
//
//	case ui.NotifyExpiredMsg:
//	    m.notes = m.notes.Expire(msg)
//	    return m, nil
//	// …then, on some action:
//	m.notes, cmd = m.notes.Push(ui.KindSuccess, "Saved")
//	// …and in View, over the finished frame:
//	return m.notes.Overlay(frame, width, height)

// Notification is one message, live or historical.
type Notification struct {
	ID      int
	Kind    Kind
	Message string
	At      time.Time
	// TTL is how long this notice stays on screen. Zero means the
	// Notifier's default for its severity.
	TTL time.Duration
}

// NotifyExpiredMsg retires one notification. Screens forward it to Expire.
type NotifyExpiredMsg struct{ ID int }

// Notifier owns the live toast stack and the history behind it.
type Notifier struct {
	live    []Notification
	history []Notification
	nextID  int

	// MaxVisible caps the stack. Beyond it the oldest is dropped from view
	// (but kept in history) — a screen buried under toasts is unusable, and
	// the pile is usually a bug in the caller anyway.
	MaxVisible int
	// MaxHistory caps retained notices.
	MaxHistory int
	// Anchor places the stack. Bottom-right by default: it's the corner
	// least likely to cover the thing the user just acted on.
	Anchor lipgloss.Position
}

// NewNotifier returns a notifier with sensible caps.
func NewNotifier() Notifier {
	return Notifier{MaxVisible: 3, MaxHistory: 50, Anchor: lipgloss.Right}
}

// defaultTTL scales with severity: an error should outlast a confirmation,
// because the user has to do something about it.
func defaultTTL(kind Kind) time.Duration {
	switch kind {
	case KindDanger:
		return 8 * time.Second
	case KindWarning:
		return 6 * time.Second
	default:
		return 4 * time.Second
	}
}

// Push adds a notification and returns the command that will expire it.
// The command must be returned from Update, or the toast never leaves.
func (n Notifier) Push(kind Kind, message string) (Notifier, tea.Cmd) {
	return n.PushFor(kind, message, 0)
}

// PushFor is Push with an explicit lifetime. Pass a very long TTL for a
// notice the user must dismiss themselves.
func (n Notifier) PushFor(kind Kind, message string, ttl time.Duration) (Notifier, tea.Cmd) {
	if strings.TrimSpace(message) == "" {
		return n, nil
	}
	if ttl <= 0 {
		ttl = defaultTTL(kind)
	}
	if n.MaxVisible <= 0 {
		n = NewNotifier()
	}

	n.nextID++
	note := Notification{ID: n.nextID, Kind: kind, Message: message, At: time.Now(), TTL: ttl}

	n.live = append(append([]Notification(nil), n.live...), note)
	for len(n.live) > n.MaxVisible {
		n.live = n.live[1:]
	}

	// Info is the "it worked, carry on" level; keeping it would turn the
	// history into noise and bury the entries that matter.
	if kind != KindInfo {
		n.history = append(append([]Notification(nil), n.history...), note)
		for len(n.history) > max(1, n.MaxHistory) {
			n.history = n.history[1:]
		}
	}

	id := note.ID
	return n, tea.Tick(ttl, func(time.Time) tea.Msg { return NotifyExpiredMsg{ID: id} })
}

// Expire retires the notification named by msg. Unknown ids are ignored, so
// a screen can forward every NotifyExpiredMsg it sees without checking.
func (n Notifier) Expire(msg NotifyExpiredMsg) Notifier {
	kept := make([]Notification, 0, len(n.live))
	for _, note := range n.live {
		if note.ID != msg.ID {
			kept = append(kept, note)
		}
	}
	n.live = kept
	return n
}

// Clear dismisses everything on screen, leaving the history intact. Bind it
// to esc: a user who wants the screen back should get it in one keypress.
func (n Notifier) Clear() Notifier {
	n.live = nil
	return n
}

// Live returns the notifications currently on screen, oldest first.
func (n Notifier) Live() []Notification { return n.live }

// History returns retained notifications, newest first, for a notification
// centre or an inbox screen.
func (n Notifier) History() []Notification {
	out := make([]Notification, 0, len(n.history))
	for index := len(n.history) - 1; index >= 0; index-- {
		out = append(out, n.history[index])
	}
	return out
}

// Unread counts warnings and errors in the history — the number worth
// putting in a status bar badge.
func (n Notifier) Unread() int {
	count := 0
	for _, note := range n.history {
		if note.Kind == KindWarning || note.Kind == KindDanger {
			count++
		}
	}
	return count
}

// Stack renders the live toasts as a right-aligned block, newest last.
// Returns an empty string when nothing is live, so a caller can skip the
// overlay entirely.
func (n Notifier) Stack(width int) string {
	if len(n.live) == 0 || width <= 0 {
		return ""
	}
	lines := make([]string, 0, len(n.live))
	for _, note := range n.live {
		toast := Toast(note.Kind, note.Message, width)
		pad := width - lipgloss.Width(toast)
		if pad > 0 && n.Anchor == lipgloss.Right {
			toast = strings.Repeat(" ", pad) + toast
		}
		lines = append(lines, toast)
	}
	return strings.Join(lines, "\n")
}

// Overlay draws the live stack over an already-rendered frame, replacing
// the rows it covers rather than pushing them down — a toast must never
// reflow the screen underneath it, or the thing the user was reading moves
// as they read it.
//
// frame must already be an exact width x height rectangle; the result is
// too.
func (n Notifier) Overlay(frame string, width, height int) string {
	if len(n.live) == 0 {
		return frame
	}
	stack := n.Stack(max(1, width-4))
	if stack == "" {
		return frame
	}

	rows := strings.Split(ClampBlock(frame, width, height), "\n")
	toasts := strings.Split(stack, "\n")

	// Sit two rows above the bottom, clear of the footer hint.
	top := height - len(toasts) - 2
	if top < 0 {
		top = 0
	}
	for index, toast := range toasts {
		at := top + index
		if at < 0 || at >= len(rows) {
			continue
		}
		rows[at] = FitLine("  "+toast, width)
	}
	return strings.Join(rows, "\n")
}

// HistoryList renders retained notifications for an inbox pane: severity
// glyph, relative age, message.
func (n Notifier) HistoryList(width, height int) string {
	notes := n.History()
	if len(notes) == 0 {
		return EmptyState("◔", "Nothing to report",
			"warnings and errors are kept here", width, height)
	}

	lines := make([]string, 0, len(notes))
	for _, note := range notes {
		marker := lipgloss.NewStyle().Foreground(note.Kind.Color()).Render(note.Kind.Glyph())
		age := SubtitleStyle.Render(RelativeAge(note.At))
		left := marker + " " + ValueStyle.Render(note.Message)
		lines = append(lines, StatusBar(left, age, width))
	}
	return ClampBlock(strings.Join(lines, "\n"), width, height)
}

// RelativeAge formats a timestamp as a short, glanceable age. Anything
// older than a day is a date, because "37h ago" makes nobody wiser.
func RelativeAge(at time.Time) string {
	elapsed := time.Since(at)
	switch {
	case elapsed < 5*time.Second:
		return "just now"
	case elapsed < time.Minute:
		return fmt.Sprintf("%ds ago", int(elapsed.Seconds()))
	case elapsed < time.Hour:
		return fmt.Sprintf("%dm ago", int(elapsed.Minutes()))
	case elapsed < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(elapsed.Hours()))
	default:
		return at.Format("2 Jan")
	}
}
