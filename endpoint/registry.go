package endpoint

import (
	"strings"

	"github.com/wlame/rls/config"
)

// Registry maps URL paths to Endpoints.
type Registry struct {
	endpoints map[string]*Endpoint
}

// NewRegistry creates an Endpoint for each config and returns a Registry.
func NewRegistry(cfgs []config.EndpointConfig) (*Registry, error) {
	r := &Registry{endpoints: make(map[string]*Endpoint, len(cfgs))}
	for _, cfg := range cfgs {
		ep, err := New(cfg)
		if err != nil {
			r.StopAll()
			return nil, err
		}
		r.endpoints[cfg.Path] = ep
	}
	return r, nil
}

// Match finds the Endpoint for the given request path.
// Exact match is preferred; falls back to the longest prefix match.
// Returns (nil, false) if no configured endpoint covers the path.
func (r *Registry) Match(path string) (*Endpoint, bool) {
	// Exact match.
	if ep, ok := r.endpoints[path]; ok {
		return ep, true
	}

	// Longest prefix match.
	var best *Endpoint
	bestLen := 0
	for prefix, ep := range r.endpoints {
		if prefix == "/" {
			continue // handle "/" as fallback below
		}
		if strings.HasPrefix(path, prefix) && len(prefix) > bestLen {
			best = ep
			bestLen = len(prefix)
		}
	}
	if best != nil {
		return best, true
	}

	// Fall back to "/" if configured.
	if ep, ok := r.endpoints["/"]; ok {
		return ep, true
	}

	return nil, false
}

// StopAll stops all endpoint dispatchers.
func (r *Registry) StopAll() {
	for _, ep := range r.endpoints {
		ep.Stop()
	}
}
