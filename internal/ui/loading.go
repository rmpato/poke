package ui

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

type taskResult[T any] struct {
	value T
	err   error
}

type loadingModel[T any] struct {
	label   string
	spinner spinner.Model
	task    func() (T, error)
	result  taskResult[T]
	done    bool
}

func (m loadingModel[T]) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, func() tea.Msg {
		value, err := m.task()
		return taskResult[T]{value: value, err: err}
	})
}

func (m loadingModel[T]) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case taskResult[T]:
		m.result = msg
		m.done = true
		return m, tea.Quit
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m loadingModel[T]) View() string {
	if m.done {
		return ""
	}
	return fmt.Sprintf("  %s %s\n", m.spinner.View(), m.label)
}

// Load runs task with animated spinner feedback when stderr is an
// interactive terminal, and runs it plain (no UI) otherwise — e.g. when
// output is piped or running in CI.
func Load[T any](label string, task func() (T, error)) (T, error) {
	if !term.IsTerminal(int(os.Stderr.Fd())) {
		return task()
	}

	s := spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(Primary)),
	)
	model := loadingModel[T]{label: label, spinner: s, task: task}
	program := tea.NewProgram(model, tea.WithOutput(os.Stderr))
	final, err := program.Run()
	if err != nil {
		var zero T
		return zero, err
	}
	return final.(loadingModel[T]).result.value, final.(loadingModel[T]).result.err
}
