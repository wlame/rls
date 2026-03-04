package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/wlame/rls/endpoint"
)

// serverEventMsg wraps an endpoint.Event for delivery as a Bubble Tea message.
type serverEventMsg struct{ ev endpoint.Event }

// tickMsg is sent every 100ms to refresh dot colours.
type tickMsg struct{}

// waitForEvent returns a Cmd that blocks until the next endpoint.Event is available.
// It is re-issued after every serverEventMsg to keep the event loop running.
func waitForEvent(ch <-chan endpoint.Event) tea.Cmd {
	return func() tea.Msg {
		return serverEventMsg{ev: <-ch}
	}
}
