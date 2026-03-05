package endpoint

import (
	"path"
	"sort"
	"sync"

	"github.com/wlame/rls/config"
)

// EndpointInfo is a snapshot of an endpoint's state.
type EndpointInfo struct {
	Config   config.EndpointConfig
	QueueLen int
}

// Registry maps URL paths to Endpoints.
type Registry struct {
	mu           sync.RWMutex
	endpoints    map[string]*Endpoint
	opts         []Option
	maxDynamic   int
	dynamicCount int
}

// RegistryOption configures a Registry.
type RegistryOption func(*Registry)

// WithMaxDynamic sets the cap on dynamically created endpoints.
func WithMaxDynamic(n int) RegistryOption {
	return func(r *Registry) {
		r.maxDynamic = n
	}
}

// NewRegistry creates an Endpoint for each config and returns a Registry.
// Endpoint Options are forwarded to every Endpoint.
// RegistryOptions configure the registry itself.
func NewRegistry(cfgs []config.EndpointConfig, opts ...Option) (*Registry, error) {
	return NewRegistryWithOpts(cfgs, nil, opts...)
}

// NewRegistryWithOpts creates a Registry with both registry and endpoint options.
func NewRegistryWithOpts(cfgs []config.EndpointConfig, regOpts []RegistryOption, epOpts ...Option) (*Registry, error) {
	r := &Registry{
		endpoints:  make(map[string]*Endpoint, len(cfgs)),
		opts:       epOpts,
		maxDynamic: 1000,
	}
	for _, ro := range regOpts {
		ro(r)
	}

	for _, cfg := range cfgs {
		ep, err := New(cfg, epOpts...)
		if err != nil {
			r.StopAll()
			return nil, err
		}
		r.endpoints[cfg.Path] = ep
	}
	return r, nil
}

// Match finds the Endpoint for the given request path.
// If no exact match exists, a dynamic endpoint is created inheriting config
// from the nearest configured ancestor via parent-path walking.
func (r *Registry) Match(reqPath string) (*Endpoint, bool) {
	// Fast path: exact match under read lock.
	r.mu.RLock()
	if ep, ok := r.endpoints[reqPath]; ok {
		r.mu.RUnlock()
		return ep, true
	}
	r.mu.RUnlock()

	// Slow path: acquire write lock for dynamic creation.
	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check: another goroutine may have created it.
	if ep, ok := r.endpoints[reqPath]; ok {
		return ep, true
	}

	// Walk parent paths to find nearest configured ancestor.
	parent := r.findParentLocked(reqPath)
	if parent == nil {
		return nil, false
	}

	// Check dynamic cap.
	if r.dynamicCount >= r.maxDynamic {
		return parent, true
	}

	// Create dynamic endpoint inheriting from parent.
	childCfg := config.InheritFrom(
		config.EndpointConfig{Path: reqPath, Dynamic: true},
		parent.cfg,
	)
	ep, err := New(childCfg, r.opts...)
	if err != nil {
		return parent, true
	}

	r.endpoints[reqPath] = ep
	r.dynamicCount++
	return ep, true
}

// findParentLocked walks parent paths from reqPath up to "/" looking for
// a registered endpoint. Must be called with r.mu held.
func (r *Registry) findParentLocked(reqPath string) *Endpoint {
	p := reqPath
	for {
		p = path.Dir(p)
		if ep, ok := r.endpoints[p]; ok {
			return ep
		}
		if p == "/" || p == "." {
			break
		}
	}
	// Check root fallback.
	if ep, ok := r.endpoints["/"]; ok {
		return ep
	}
	return nil
}

// Snapshot returns a sorted slice of all endpoint info.
func (r *Registry) Snapshot() []EndpointInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	infos := make([]EndpointInfo, 0, len(r.endpoints))
	for _, ep := range r.endpoints {
		infos = append(infos, EndpointInfo{
			Config:   ep.cfg,
			QueueLen: ep.QueueLen(),
		})
	}
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Config.Path < infos[j].Config.Path
	})
	return infos
}

// QueueDepths returns the current queue depth for every endpoint, keyed by path.
func (r *Registry) QueueDepths() map[string]int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	depths := make(map[string]int, len(r.endpoints))
	for path, ep := range r.endpoints {
		depths[path] = ep.QueueLen()
	}
	return depths
}

// StopAll stops all endpoint dispatchers.
func (r *Registry) StopAll() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, ep := range r.endpoints {
		ep.Stop()
	}
}
