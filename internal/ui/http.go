package ui

import "github.com/charmbracelet/lipgloss"

// The HTTP vocabulary is part of pogo's theme, not something a screen invents.
// Colour in pogo carries exactly two meanings — how much damage a request can
// do, and how it came back — and both are decided here so every screen says
// the same thing with the same colour (SYSTEM_DESIGN.md §4.3).

// MethodColor colours an HTTP method by how much damage it can do: reads are
// calm, writes are warm, deletes are loud.
func MethodColor(method string) lipgloss.TerminalColor {
	switch method {
	case "GET":
		return Primary
	case "POST":
		return Success
	case "PUT", "PATCH":
		return Warning
	case "DELETE":
		return Danger
	case "HEAD", "OPTIONS":
		return Muted
	default:
		return Text
	}
}

// MethodStyle is MethodColor as a style, for a cell that owns its own line.
func MethodStyle(method string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(MethodColor(method)).Bold(true)
}

// StatusKind maps a response code onto the four severities the whole kit
// shares, so a status reads the same in a row, a banner and a toast.
//
// A request that produced no status at all is a failure, and reads as one:
// code 0 means curl never got an answer.
func StatusKind(code int) Kind {
	switch {
	case code == 0:
		return KindDanger
	case code < 200:
		return KindInfo
	case code < 300:
		return KindSuccess
	case code < 400:
		return KindInfo
	case code < 500:
		return KindWarning
	default:
		return KindDanger
	}
}

// StatusColor colours a response by class.
func StatusColor(code int) lipgloss.TerminalColor { return StatusKind(code).Color() }

// StatusStyle is StatusColor as a style.
func StatusStyle(code int) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(StatusColor(code))
}
