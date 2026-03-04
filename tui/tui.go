// Package tui provides an interactive terminal UI for rls.
// It is activated by the --interactive flag and runs alongside the HTTP server.
package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/wlame/rls/config"
	"github.com/wlame/rls/endpoint"
)

// Run starts the interactive Bubble Tea program and blocks until the user quits.
// cfg populates the endpoint list and derives the server address.
// events carries live rate-limiting events emitted by the running server.
func Run(cfg *config.Config, events <-chan endpoint.Event) error {
	m := NewModel(cfg, events)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return nil
}
