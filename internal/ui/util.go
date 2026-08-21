package ui

import (
	"os/exec"
	"runtime"
)

// Fallback returns value, or fallbackValue when value is empty.
func Fallback(value, fallbackValue string) string {
	if value == "" {
		return fallbackValue
	}
	return value
}

// ShareWidth splits total across columns, handing the remainder to the
// first columns so a row of cards spans its container exactly.
func ShareWidth(total, columns int) []int {
	if columns <= 0 {
		return nil
	}
	base := total / columns
	remainder := total - base*columns
	widths := make([]int, columns)
	for index := range widths {
		widths[index] = base
		if index < remainder {
			widths[index]++
		}
	}
	return widths
}

// Truncate shortens name to at most n bytes, appending an ellipsis when it
// doesn't fit. Intended for short, ASCII-ish labels; prefer ansi.Truncate
// for strings that may already carry styling.
func Truncate(name string, n int) string {
	if len(name) <= n {
		return name
	}
	if n <= 0 {
		return ""
	}
	return name[:n-1] + "…"
}

// OpenURLCommand returns the OS-appropriate command to open url in the
// default browser. The caller is responsible for running it.
func OpenURLCommand(url string) *exec.Cmd {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url)
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return exec.Command("xdg-open", url)
	}
}
