package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Host string `yaml:"host" json:"host"`
	Port int    `yaml:"port" json:"port"`
}

// Defaults holds fallback values applied to endpoints that omit a field.
type Defaults struct {
	Scheduler           string  `yaml:"scheduler" json:"scheduler"`
	Algorithm           string  `yaml:"algorithm" json:"algorithm"`
	Unit                string  `yaml:"unit" json:"unit"`
	MaxQueueSize        int     `yaml:"max_queue_size" json:"max_queue_size"`
	Overflow            string  `yaml:"overflow" json:"overflow"`
	QueueTimeout        float64 `yaml:"queue_timeout" json:"queue_timeout"`
	LatencyCompensation float64 `yaml:"latency_compensation" json:"latency_compensation"`
	MaxDynamicEndpoints int     `yaml:"max_dynamic_endpoints" json:"max_dynamic_endpoints"`
	TokensPerWindow     int     `yaml:"tokens_per_window" json:"tokens_per_window"`
	DefaultTokens       int     `yaml:"default_tokens" json:"default_tokens"`
}

// EndpointConfig describes a single rate-limited endpoint.
type EndpointConfig struct {
	Path          string  `yaml:"path" json:"path"`
	Rate          float64 `yaml:"rate" json:"rate"`
	Unit          string  `yaml:"unit" json:"unit"`
	Scheduler     string  `yaml:"scheduler" json:"scheduler"`
	Algorithm     string  `yaml:"algorithm" json:"algorithm"`
	MaxQueueSize  int     `yaml:"max_queue_size" json:"max_queue_size"`
	Overflow      string  `yaml:"overflow" json:"overflow"`
	BurstSize     int     `yaml:"burst_size" json:"burst_size"`
	WindowSeconds int     `yaml:"window_seconds" json:"window_seconds"`
	QueueTimeout        float64 `yaml:"queue_timeout" json:"queue_timeout"`
	LatencyCompensation float64 `yaml:"latency_compensation" json:"latency_compensation"`
	TokensPerWindow     int     `yaml:"tokens_per_window" json:"tokens_per_window"`
	DefaultTokens       int     `yaml:"default_tokens" json:"default_tokens"`
	Dynamic             bool    `yaml:"-" json:"-"`
}

// Config is the top-level configuration.
type Config struct {
	Server    ServerConfig     `yaml:"server" json:"server"`
	Defaults  Defaults         `yaml:"defaults" json:"defaults"`
	Endpoints []EndpointConfig `yaml:"endpoints" json:"endpoints"`
}

// systemDefaults are the hard-coded fallbacks when nothing is configured.
var systemDefaults = Defaults{
	Scheduler:    "fifo",
	Algorithm:    "strict",
	Unit:         "rps",
	MaxQueueSize: 1000,
	Overflow:     "reject",
}

// Load reads a YAML config file and returns a populated Config.
// If the file is empty or has no endpoints, the root "/" endpoint is auto-inserted.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}

	if err := applyServerDefaults(&cfg); err != nil {
		return nil, err
	}
	ApplyDefaults(&cfg)

	if err := validateEndpoints(cfg.Endpoints); err != nil {
		return nil, err
	}
	return &cfg, nil
}

var (
	validAlgorithms = map[string]bool{"strict": true, "token_bucket": true, "sliding_window": true, "token_window": true}
	validSchedulers = map[string]bool{"fifo": true, "lifo": true, "priority": true, "random": true}
	validUnits      = map[string]bool{"rps": true, "rpm": true}
)

// validateEndpoints checks per-endpoint constraints.
func validateEndpoints(endpoints []EndpointConfig) error {
	for _, ep := range endpoints {
		if !validAlgorithm(ep.Algorithm) {
			return fmt.Errorf("endpoint %q: unknown algorithm %q; valid: strict, token_bucket, sliding_window, token_window", ep.Path, ep.Algorithm)
		}
		if !validScheduler(ep.Scheduler) {
			return fmt.Errorf("endpoint %q: unknown scheduler %q; valid: fifo, lifo, priority, random", ep.Path, ep.Scheduler)
		}
		if !validUnit(ep.Unit) {
			return fmt.Errorf("endpoint %q: unknown unit %q; valid: rps, rpm", ep.Path, ep.Unit)
		}
		if ep.Algorithm != "token_window" && ep.Rate <= 0 {
			return fmt.Errorf("endpoint %q: rate must be > 0, got %g", ep.Path, ep.Rate)
		}
		if ep.MaxQueueSize <= 0 {
			return fmt.Errorf("endpoint %q: max_queue_size must be > 0, got %d", ep.Path, ep.MaxQueueSize)
		}
		if ep.QueueTimeout < 0 {
			return fmt.Errorf("endpoint %q: queue_timeout must be >= 0, got %g", ep.Path, ep.QueueTimeout)
		}
		if ep.Algorithm == "token_window" {
			if ep.TokensPerWindow <= 0 {
				return fmt.Errorf("endpoint %q: token_window requires tokens_per_window > 0", ep.Path)
			}
			if ep.WindowSeconds <= 0 {
				return fmt.Errorf("endpoint %q: token_window requires window_seconds > 0", ep.Path)
			}
			if ep.DefaultTokens > ep.TokensPerWindow {
				return fmt.Errorf("endpoint %q: default_tokens (%d) exceeds tokens_per_window (%d)", ep.Path, ep.DefaultTokens, ep.TokensPerWindow)
			}
		}
	}
	return nil
}

func validAlgorithm(s string) bool { return validAlgorithms[s] }
func validScheduler(s string) bool { return validSchedulers[s] }
func validUnit(s string) bool      { return validUnits[s] }

// applyServerDefaults fills in server-level defaults and validates port range.
func applyServerDefaults(cfg *Config) error {
	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		return fmt.Errorf("server port %d out of range [1, 65535]", cfg.Server.Port)
	}
	return nil
}

// ApplyDefaults fills missing EndpointConfig fields from Defaults (with system fallback),
// and auto-inserts a root "/" endpoint if none exists.
func ApplyDefaults(cfg *Config) {
	// merge user Defaults with system defaults
	d := mergeWithSystem(cfg.Defaults)

	// ensure root endpoint exists
	hasRoot := false
	for _, ep := range cfg.Endpoints {
		if ep.Path == "/" {
			hasRoot = true
			break
		}
	}
	if !hasRoot {
		cfg.Endpoints = append([]EndpointConfig{
			{
				Path:         "/",
				Rate:         1,
				Unit:         d.Unit,
				Scheduler:    d.Scheduler,
				Algorithm:    d.Algorithm,
				MaxQueueSize: d.MaxQueueSize,
				Overflow:     d.Overflow,
			},
		}, cfg.Endpoints...)
	}

	// fill missing fields per endpoint
	for i := range cfg.Endpoints {
		ep := &cfg.Endpoints[i]
		if ep.Rate == 0 {
			ep.Rate = 1
		}
		if ep.Unit == "" {
			ep.Unit = d.Unit
		}
		if ep.Scheduler == "" {
			ep.Scheduler = d.Scheduler
		}
		if ep.Algorithm == "" {
			ep.Algorithm = d.Algorithm
		}
		if ep.MaxQueueSize == 0 {
			ep.MaxQueueSize = d.MaxQueueSize
		}
		if ep.Overflow == "" {
			ep.Overflow = d.Overflow
		}
		if ep.QueueTimeout == 0 {
			ep.QueueTimeout = d.QueueTimeout
		}
		if ep.LatencyCompensation == 0 {
			ep.LatencyCompensation = d.LatencyCompensation
		}
		if ep.TokensPerWindow == 0 {
			ep.TokensPerWindow = d.TokensPerWindow
		}
		if ep.DefaultTokens == 0 {
			ep.DefaultTokens = d.DefaultTokens
		}
		// token_window: default_tokens must be at least 1
		if ep.Algorithm == "token_window" && ep.DefaultTokens == 0 {
			ep.DefaultTokens = 1
		}
	}
}

// InheritFrom fills zero-value fields in child from parent.
// Path and Dynamic are always preserved from child.
func InheritFrom(child, parent EndpointConfig) EndpointConfig {
	if child.Rate == 0 {
		child.Rate = parent.Rate
	}
	if child.Unit == "" {
		child.Unit = parent.Unit
	}
	if child.Scheduler == "" {
		child.Scheduler = parent.Scheduler
	}
	if child.Algorithm == "" {
		child.Algorithm = parent.Algorithm
	}
	if child.MaxQueueSize == 0 {
		child.MaxQueueSize = parent.MaxQueueSize
	}
	if child.Overflow == "" {
		child.Overflow = parent.Overflow
	}
	if child.BurstSize == 0 {
		child.BurstSize = parent.BurstSize
	}
	if child.WindowSeconds == 0 {
		child.WindowSeconds = parent.WindowSeconds
	}
	if child.QueueTimeout == 0 {
		child.QueueTimeout = parent.QueueTimeout
	}
	if child.LatencyCompensation == 0 {
		child.LatencyCompensation = parent.LatencyCompensation
	}
	if child.TokensPerWindow == 0 {
		child.TokensPerWindow = parent.TokensPerWindow
	}
	if child.DefaultTokens == 0 {
		child.DefaultTokens = parent.DefaultTokens
	}
	return child
}

func mergeWithSystem(d Defaults) Defaults {
	if d.Scheduler == "" {
		d.Scheduler = systemDefaults.Scheduler
	}
	if d.Algorithm == "" {
		d.Algorithm = systemDefaults.Algorithm
	}
	if d.Unit == "" {
		d.Unit = systemDefaults.Unit
	}
	if d.MaxQueueSize == 0 {
		d.MaxQueueSize = systemDefaults.MaxQueueSize
	}
	if d.Overflow == "" {
		d.Overflow = systemDefaults.Overflow
	}
	return d
}
