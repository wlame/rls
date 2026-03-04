// Package tui provides an interactive terminal UI for rls.
// It is activated by the --interactive flag and runs alongside the HTTP server.
package tui

import (
	"fmt"

	"github.com/wlame/rls/config"
	"github.com/wlame/rls/endpoint"
)

// Run starts the interactive TUI program. It blocks until the user quits.
// cfg is used to populate the endpoint list and derive the server address.
// events carries live rate-limiting events from the running server.
func Run(cfg *config.Config, events <-chan endpoint.Event) error {
	fmt.Println("TUI not yet implemented")
	return nil
}
