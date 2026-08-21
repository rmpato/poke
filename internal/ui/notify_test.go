package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestNotifierPushReturnsAnExpiryCommand(t *testing.T) {
	// A short TTL keeps the test fast: invoking the returned command runs
	// the tick, which otherwise waits out the real lifetime.
	n, cmd := NewNotifier().PushFor(KindSuccess, "Saved", time.Millisecond)
	if len(n.Live()) != 1 {
		t.Fatalf("expected one live toast, got %d", len(n.Live()))
	}
	if cmd == nil {
		t.Fatal("Push must return a command, or the toast never expires")
	}
	msg, ok := cmd().(NotifyExpiredMsg)
	if !ok {
		t.Fatalf("expected NotifyExpiredMsg, got %T", cmd())
	}
	if n = n.Expire(msg); len(n.Live()) != 0 {
		t.Fatalf("expiry should retire the toast, %d left", len(n.Live()))
	}
}

func TestNotifierIgnoresBlankMessages(t *testing.T) {
	n, cmd := NewNotifier().Push(KindInfo, "   ")
	if len(n.Live()) != 0 || cmd != nil {
		t.Fatal("a blank message should be dropped, not queued")
	}
}

// A new notice must never silently replace one nobody has read yet.
func TestNotifierStacksRatherThanReplacing(t *testing.T) {
	n := NewNotifier()
	n, _ = n.Push(KindInfo, "first")
	n, _ = n.Push(KindSuccess, "second")
	if len(n.Live()) != 2 {
		t.Fatalf("expected both toasts live, got %d", len(n.Live()))
	}
	if n.Live()[0].Message != "first" || n.Live()[1].Message != "second" {
		t.Fatalf("expected oldest first, got %v", n.Live())
	}
}

func TestNotifierCapsTheVisibleStack(t *testing.T) {
	n := NewNotifier()
	n.MaxVisible = 2
	for _, msg := range []string{"a", "b", "c", "d"} {
		n, _ = n.Push(KindWarning, msg)
	}
	if len(n.Live()) != 2 {
		t.Fatalf("expected 2 visible, got %d", len(n.Live()))
	}
	if n.Live()[1].Message != "d" {
		t.Fatalf("newest should survive the cap, got %v", n.Live())
	}
	// Everything above info is still recoverable from the history.
	if len(n.History()) != 4 {
		t.Fatalf("expected 4 in history, got %d", len(n.History()))
	}
}

func TestNotifierExpiryIsIndependentOfOrder(t *testing.T) {
	n := NewNotifier()
	var first, second NotifyExpiredMsg
	n, cmd := n.PushFor(KindInfo, "first", time.Millisecond)
	first = cmd().(NotifyExpiredMsg)
	n, cmd = n.PushFor(KindInfo, "second", time.Millisecond)
	second = cmd().(NotifyExpiredMsg)

	n = n.Expire(second) // retire the newer one first
	if len(n.Live()) != 1 || n.Live()[0].Message != "first" {
		t.Fatalf("expected only 'first' to remain, got %v", n.Live())
	}
	n = n.Expire(first)
	if len(n.Live()) != 0 {
		t.Fatal("expected an empty stack")
	}
	// An id that was already retired must be harmless.
	if n = n.Expire(first); len(n.Live()) != 0 {
		t.Fatal("re-expiring must be a no-op")
	}
}

func TestNotifierHistoryIsNewestFirstAndSkipsInfo(t *testing.T) {
	n := NewNotifier()
	n, _ = n.Push(KindInfo, "chatter")
	n, _ = n.Push(KindWarning, "slow")
	n, _ = n.Push(KindDanger, "broken")

	history := n.History()
	if len(history) != 2 {
		t.Fatalf("info should not be retained, got %v", history)
	}
	if history[0].Message != "broken" {
		t.Fatalf("expected newest first, got %v", history)
	}
	if n.Unread() != 2 {
		t.Fatalf("expected 2 unread, got %d", n.Unread())
	}
}

func TestNotifierClearKeepsHistory(t *testing.T) {
	n := NewNotifier()
	n, _ = n.Push(KindDanger, "broken")
	n = n.Clear()
	if len(n.Live()) != 0 {
		t.Fatal("Clear should empty the visible stack")
	}
	if len(n.History()) != 1 {
		t.Fatal("Clear must not discard the history")
	}
}

func TestCriticalToastsOutliveConfirmations(t *testing.T) {
	if defaultTTL(KindDanger) <= defaultTTL(KindSuccess) {
		t.Fatal("an error should stay on screen longer than a confirmation")
	}
}

// A toast must never reflow the screen underneath it: the overlay replaces
// rows in place and returns the same rectangle it was given.
func TestNotifierOverlayPreservesTheRectangle(t *testing.T) {
	n := NewNotifier()
	n, _ = n.Push(KindSuccess, "Saved")
	n, _ = n.Push(KindWarning, "Careful")

	const width, height = 80, 24
	frame := ClampBlock(strings.Repeat("x", width)+"\n", width, height)
	out := n.Overlay(frame, width, height)

	lines := strings.Split(out, "\n")
	if len(lines) != height {
		t.Fatalf("overlay changed the height: %d", len(lines))
	}
	for index, line := range lines {
		if got := ansi.StringWidth(line); got != width {
			t.Fatalf("line %d is %d cells, want %d", index, got, width)
		}
	}
	if !strings.Contains(out, "Saved") {
		t.Fatal("expected the toast text in the frame")
	}
}

func TestNotifierOverlayIsAPassThroughWhenIdle(t *testing.T) {
	frame := ClampBlock("hello", 40, 6)
	if got := NewNotifier().Overlay(frame, 40, 6); got != frame {
		t.Fatal("an idle notifier must return the frame untouched")
	}
}

func TestNotifierStackIsRightAligned(t *testing.T) {
	n := NewNotifier()
	n.Anchor = lipgloss.Right
	n, _ = n.Push(KindInfo, "hi")
	stack := n.Stack(60)
	if !strings.HasPrefix(stack, " ") {
		t.Fatalf("expected right alignment padding, got %q", stack)
	}
}

func TestHistoryListIsExactRectangle(t *testing.T) {
	n := NewNotifier()
	n, _ = n.Push(KindDanger, "Rollout failed")

	for _, size := range [][2]int{{30, 4}, {70, 10}} {
		block := n.HistoryList(size[0], size[1])
		assertRectangle(t, "HistoryList", block, size[0], size[1])
	}
	// The empty case must fill its box too, not collapse.
	empty := NewNotifier().HistoryList(50, 8)
	assertRectangle(t, "HistoryList empty", empty, 50, 8)
}

func TestRelativeAge(t *testing.T) {
	now := time.Now()
	cases := map[time.Duration]string{
		time.Second:      "just now",
		30 * time.Second: "30s ago",
		5 * time.Minute:  "5m ago",
		3 * time.Hour:    "3h ago",
		72 * time.Hour:   now.Add(-72 * time.Hour).Format("2 Jan"),
		400 * time.Hour:  now.Add(-400 * time.Hour).Format("2 Jan"),
	}
	for elapsed, want := range cases {
		if got := RelativeAge(now.Add(-elapsed)); got != want {
			t.Errorf("RelativeAge(-%v) = %q, want %q", elapsed, got, want)
		}
	}
}
