package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gigatone/biblelearn/internal/engine"
)

// tuiRun launches the interactive dark-mode terminal interface built on
// Bubble Tea + lipgloss.
func tuiRun(e *engine.Engine) error {
	m, err := newAppModel(e)
	if err != nil {
		return err
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return nil
}
