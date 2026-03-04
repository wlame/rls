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
// Any Options provided are forwarded to every Endpoint.
func NewRegistry(cfgs []config.EndpointConfig, opts ...Option) (*Registry, error) {
	r := &Registry{endpoints: make(map[string]*Endpoint, len(cfgs))}
	for _, cfg := range cfgs {
		ep, err := New(cfg, opts...)
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

// QueueDepths returns the current queue depth for every endpoint, keyed by path.
func (r *Registry) QueueDepths() map[string]int {
	depths := make(map[string]int, len(r.endpoints))
	for path, ep := range r.endpoints {
		depths[path] = ep.QueueLen()
	}
	return depths
}

// StopAll stops all endpoint dispatchers.
func (r *Registry) StopAll() {
	for _, ep := range r.endpoints {
		ep.Stop()
	}
}
