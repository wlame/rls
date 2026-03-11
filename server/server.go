package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/wlame/rls/config"
	"github.com/wlame/rls/endpoint"
)

// Server wraps the HTTP server and the endpoint registry.
type Server struct {
	http           *http.Server
	registry       *endpoint.Registry
	activeRequests sync.WaitGroup
}

// New creates a Server from cfg. Any endpoint.Option values are forwarded to every Endpoint.
func New(cfg config.Config, epOpts ...endpoint.Option) (*Server, error) {
	var regOpts []endpoint.RegistryOption
	if cfg.Defaults.MaxDynamicEndpoints > 0 {
		regOpts = append(regOpts, endpoint.WithMaxDynamic(cfg.Defaults.MaxDynamicEndpoints))
	}
	registry, err := endpoint.NewRegistryWithOpts(cfg.Endpoints, regOpts, epOpts...)
	if err != nil {
		return nil, fmt.Errorf("build registry: %w", err)
	}

	s := &Server{
		registry: registry,
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
		s.activeRequests.Add(1)
		defer s.activeRequests.Done()
		ep.Handle(w, r)
	})

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	s.http = &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	return s, nil
}

// Start begins listening and blocks until the server is shut down.
// Call Shutdown() from another goroutine to stop it gracefully.
func (s *Server) Start() error {
	err := s.http.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Shutdown gracefully drains the server with a 30-second timeout.
// 1. Stop accepting new connections (http.Shutdown).
// 2. Wait for in-flight request handlers to finish.
// 3. Stop dispatchers and limiters (StopAll).
func (s *Server) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := s.http.Shutdown(ctx)
	s.activeRequests.Wait()
	s.registry.StopAll()
	return err
}

// Registry returns the endpoint registry for external access (e.g. attach state).
func (s *Server) Registry() *endpoint.Registry {
	return s.registry
}

// Handler returns the server's http.Handler for use in tests.
func (s *Server) Handler() http.Handler {
	return s.http.Handler
}
