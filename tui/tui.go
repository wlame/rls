// Package tui provides an interactive terminal UI for rls.
// It is activated by the --interactive flag and runs alongside the HTTP server.
package tui

import (
	"fmt"
	"io"
	"strings"
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

// LogSink returns an io.Writer suitable for log.SetOutput and a read channel
// that feeds log lines into the TUI log panel.
// bufSize controls how many lines can be buffered before drops occur.
func LogSink(bufSize int) (io.Writer, <-chan string) {
	ch := make(chan string, bufSize)
	return &logWriter{ch: ch}, ch
}

type logWriter struct{ ch chan<- string }

func (w *logWriter) Write(p []byte) (int, error) {
	for _, line := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		if line == "" {
			continue
		}
		select {
		case w.ch <- line:
		default: // drop when full; never block the logger
		}
	}
	return len(p), nil
}

// Run starts the interactive Bubble Tea program and blocks until the user quits.
// cfg populates the endpoint list and derives the server address.
// events carries live rate-limiting events emitted by the running server.
// logCh receives lines written via LogSink; pass nil to hide the log panel.
func Run(cfg *config.Config, events <-chan endpoint.Event, thresholds DotThresholds, logCh <-chan string, attachedPID int) error {
	m := NewModel(cfg, events, thresholds, logCh, attachedPID)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return nil
}
