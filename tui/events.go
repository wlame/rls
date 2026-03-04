package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/wlame/rls/endpoint"
)

// serverEventMsg wraps an endpoint.Event for delivery as a Bubble Tea message.
type serverEventMsg struct{ ev endpoint.Event }

// tickMsg is sent every 100ms to refresh dot colours.
type tickMsg struct{}

// logLineMsg carries a single log line to the TUI log panel.
type logLineMsg struct{ line string }

// disconnectedMsg signals that the remote rls process has disconnected.
type disconnectedMsg struct{}

// waitForEvent returns a Cmd that blocks until the next endpoint.Event is available.
// If the channel is closed, it returns a disconnectedMsg.
func waitForEvent(ch <-chan endpoint.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return disconnectedMsg{}
		}
		return serverEventMsg{ev: ev}
	}
}

// waitForLog returns a Cmd that blocks until the next log line is available.
// If the channel is closed, it returns a disconnectedMsg.
// A nil channel blocks forever (no-op), which is fine for tests.
func waitForLog(ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return disconnectedMsg{}
		}
		return logLineMsg{line: line}
	}
}
