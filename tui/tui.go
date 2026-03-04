// Package tui provides an interactive terminal UI for rls.
// It is activated by the --interactive flag and runs alongside the HTTP server.
package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/wlame/rls/config"
	"github.com/wlame/rls/endpoint"
)

// DotThresholds controls when queue dot colours change from green → yellow → red.
type DotThresholds struct {
	Warn time.Duration // green below this; yellow at or above (default 2s)
	Crit time.Duration // yellow below this; red at or above (default 5s)
}

// DefaultDotThresholds returns the default colour thresholds.
func DefaultDotThresholds() DotThresholds {
	return DotThresholds{Warn: 2 * time.Second, Crit: 5 * time.Second}
}

// Run starts the interactive Bubble Tea program and blocks until the user quits.
// cfg populates the endpoint list and derives the server address.
// events carries live rate-limiting events emitted by the running server.
func Run(cfg *config.Config, events <-chan endpoint.Event, thresholds DotThresholds) error {
	m := NewModel(cfg, events, thresholds)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return nil
}
