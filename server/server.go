package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/wlame/rls/config"
	"github.com/wlame/rls/endpoint"
)

// Server wraps the HTTP server and the endpoint registry.
type Server struct {
	http     *http.Server
	registry *endpoint.Registry
}

// New creates a Server from cfg. Any endpoint.Option values are forwarded to every Endpoint.
func New(cfg config.Config, epOpts ...endpoint.Option) (*Server, error) {
	registry, err := endpoint.NewRegistry(cfg.Endpoints, epOpts...)
	if err != nil {
		return nil, fmt.Errorf("build registry: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		ep, ok := registry.Match(r.URL.Path)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
				"ok":    false,
				"error": "no endpoint configured for path",
			})
			return
		}
		ep.Handle(w, r)
	})

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	s := &Server{
		http: &http.Server{
			Addr:    addr,
			Handler: mux,
		},
		registry: registry,
	}
	return s, nil
}

// Start begins listening and blocks until the server stops.
// It handles SIGINT/SIGTERM for graceful shutdown.
func (s *Server) Start() error {
	done := make(chan error, 1)
	go func() {
		done <- s.http.ListenAndServe()
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-done:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	case <-quit:
		return s.Shutdown()
	}
}

// Shutdown gracefully drains the server with a 30-second timeout.
func (s *Server) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	s.registry.StopAll()
	return s.http.Shutdown(ctx)
}

// Handler returns the server's http.Handler for use in tests.
func (s *Server) Handler() http.Handler {
	return s.http.Handler
}
