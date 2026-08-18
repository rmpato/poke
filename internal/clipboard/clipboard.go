// Package clipboard copies text to the system clipboard.
//
// It tries the platform's clipboard tool first and falls back to an OSC 52
// escape sequence, which is what makes copying work over SSH and inside tmux --
// the case where reaching for a request you made on a remote box matters most.
package clipboard

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// ErrUnavailable is returned when no clipboard mechanism could be used.
var ErrUnavailable = errors.New("no clipboard available")

// tool is an external clipboard command and the arguments it needs.
type tool struct {
	name string
	args []string
}

// tools lists candidates in preference order for the current platform.
func tools() []tool {
	if runtime.GOOS == "darwin" {
		return []tool{{"pbcopy", nil}}
	}
	return []tool{
		{"wl-copy", nil}, // Wayland
		{"xclip", []string{"-selection", "clipboard"}}, // X11
		{"xsel", []string{"--clipboard", "--input"}},
	}
}

// Copy places text on the clipboard, reporting which mechanism succeeded so the
// UI can say "copied" honestly rather than optimistically.
func Copy(text string) (string, error) {
	for _, t := range tools() {
		path, err := exec.LookPath(t.name)
		if err != nil {
			continue
		}
		cmd := exec.Command(path, t.args...)
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return t.name, nil
		}
	}
	if err := osc52(text); err == nil {
		return "osc52", nil
	}
	return "", ErrUnavailable
}

// osc52 asks the terminal emulator itself to set the clipboard.
//
// The sequence is written to the controlling terminal rather than to stdout,
// because a Bubble Tea program owns stdout and an escape sequence injected
// there would be overwritten by the next frame.
func osc52(text string) error {
	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer func() { _ = tty.Close() }()

	encoded := base64.StdEncoding.EncodeToString([]byte(text))
	seq := fmt.Sprintf("\x1b]52;c;%s\a", encoded)

	// tmux only forwards the sequence to the outer terminal when it is wrapped
	// in a passthrough, and it needs the inner ESC doubled.
	if os.Getenv("TMUX") != "" {
		seq = "\x1bPtmux;\x1b" + strings.ReplaceAll(seq, "\x1b", "\x1b\x1b") + "\x1b\\"
	}
	_, err = tty.WriteString(seq)
	return err
}
