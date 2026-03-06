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

	applyServerDefaults(&cfg)
	ApplyDefaults(&cfg)
	return &cfg, nil
}

// applyServerDefaults fills in server-level defaults.
func applyServerDefaults(cfg *Config) {
	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
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
				Unit:         "rps",
				Scheduler:    "fifo",
				Algorithm:    "strict",
				MaxQueueSize: 1000,
				Overflow:     "reject",
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
		if ep.LatencyCompensation == 0 {
			ep.LatencyCompensation = d.LatencyCompensation
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
