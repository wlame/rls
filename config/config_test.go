package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "rls-*.yml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}

func TestLoad_ValidYAML(t *testing.T) {
	path := writeTemp(t, `
server:
  host: "127.0.0.1"
  port: 9090
defaults:
  scheduler: lifo
  unit: rpm
  max_queue_size: 500
  overflow: block
endpoints:
  - path: "/api"
    rate: 60
    algorithm: token_bucket
    burst_size: 10
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("host: got %q, want 127.0.0.1", cfg.Server.Host)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("port: got %d, want 9090", cfg.Server.Port)
	}

	// find /api endpoint (root should be auto-inserted at index 0)
	var apiEp *EndpointConfig
	for i := range cfg.Endpoints {
		if cfg.Endpoints[i].Path == "/api" {
			apiEp = &cfg.Endpoints[i]
		}
	}
	if apiEp == nil {
		t.Fatal("/api endpoint not found")
	}
	if apiEp.Scheduler != "lifo" {
		t.Errorf("scheduler: got %q, want lifo", apiEp.Scheduler)
	}
	if apiEp.Unit != "rpm" {
		t.Errorf("unit: got %q, want rpm", apiEp.Unit)
	}
	if apiEp.MaxQueueSize != 500 {
		t.Errorf("max_queue_size: got %d, want 500", apiEp.MaxQueueSize)
	}
	if apiEp.Overflow != "block" {
		t.Errorf("overflow: got %q, want block", apiEp.Overflow)
	}
	if apiEp.BurstSize != 10 {
		t.Errorf("burst_size: got %d, want 10", apiEp.BurstSize)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nonexistent.yml"))
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	path := writeTemp(t, `{invalid yaml:::`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestApplyDefaults_FillsMissingFields(t *testing.T) {
	cfg := &Config{
		Defaults: Defaults{
			Scheduler:    "priority",
			Algorithm:    "token_bucket",
			Unit:         "rpm",
			MaxQueueSize: 200,
			Overflow:     "block",
		},
		Endpoints: []EndpointConfig{
			{Path: "/", Rate: 5},
			{Path: "/other", Rate: 10, Scheduler: "random"},
		},
	}
	ApplyDefaults(cfg)

	root := cfg.Endpoints[0]
	if root.Scheduler != "fifo" {
		// root "/" was explicitly configured, should keep fifo (auto-insert skipped since / is present)
	}

	other := cfg.Endpoints[1]
	if other.Scheduler != "random" {
		t.Errorf("should preserve explicit scheduler, got %q", other.Scheduler)
	}
	if other.Algorithm != "token_bucket" {
		t.Errorf("algorithm: got %q, want token_bucket", other.Algorithm)
	}
	if other.Unit != "rpm" {
		t.Errorf("unit: got %q, want rpm", other.Unit)
	}
	if other.MaxQueueSize != 200 {
		t.Errorf("max_queue_size: got %d, want 200", other.MaxQueueSize)
	}
	if other.Overflow != "block" {
		t.Errorf("overflow: got %q, want block", other.Overflow)
	}
}

func TestApplyDefaults_AutoInsertRoot(t *testing.T) {
	cfg := &Config{
		Endpoints: []EndpointConfig{
			{Path: "/api", Rate: 10},
		},
	}
	ApplyDefaults(cfg)

	if cfg.Endpoints[0].Path != "/" {
		t.Errorf("first endpoint should be auto-inserted /, got %q", cfg.Endpoints[0].Path)
	}
	root := cfg.Endpoints[0]
	if root.Rate != 1 {
		t.Errorf("root rate: got %f, want 1", root.Rate)
	}
	if root.Unit != "rps" {
		t.Errorf("root unit: got %q, want rps", root.Unit)
	}
	if root.Scheduler != "fifo" {
		t.Errorf("root scheduler: got %q, want fifo", root.Scheduler)
	}
	if root.Algorithm != "strict" {
		t.Errorf("root algorithm: got %q, want strict", root.Algorithm)
	}
}

func TestApplyDefaults_NoAutoInsertWhenRootPresent(t *testing.T) {
	cfg := &Config{
		Endpoints: []EndpointConfig{
			{Path: "/", Rate: 5},
		},
	}
	ApplyDefaults(cfg)
	count := 0
	for _, ep := range cfg.Endpoints {
		if ep.Path == "/" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 root endpoint, got %d", count)
	}
}

func TestApplyDefaults_SystemDefaultsWhenNoUserDefaults(t *testing.T) {
	cfg := &Config{
		Endpoints: []EndpointConfig{
			{Path: "/", Rate: 3},
		},
	}
	ApplyDefaults(cfg)
	ep := cfg.Endpoints[0]
	if ep.Scheduler != "fifo" {
		t.Errorf("scheduler: got %q, want fifo", ep.Scheduler)
	}
	if ep.Algorithm != "strict" {
		t.Errorf("algorithm: got %q, want strict", ep.Algorithm)
	}
	if ep.MaxQueueSize != 1000 {
		t.Errorf("max_queue_size: got %d, want 1000", ep.MaxQueueSize)
	}
}

func TestLoad_ServerDefaults(t *testing.T) {
	path := writeTemp(t, `endpoints:
  - path: "/"
    rate: 1
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("default host: got %q, want 0.0.0.0", cfg.Server.Host)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("default port: got %d, want 8080", cfg.Server.Port)
	}
}

// --- merge tests ---

func TestMergeOverrides_Port(t *testing.T) {
	cfg := &Config{Server: ServerConfig{Host: "0.0.0.0", Port: 8080}}
	if err := MergeOverrides(cfg, map[string]string{"port": "9999"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Port != 9999 {
		t.Errorf("port: got %d, want 9999", cfg.Server.Port)
	}
}

func TestMergeOverrides_Host(t *testing.T) {
	cfg := &Config{Server: ServerConfig{Host: "0.0.0.0", Port: 8080}}
	if err := MergeOverrides(cfg, map[string]string{"host": "127.0.0.1"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("host: got %q, want 127.0.0.1", cfg.Server.Host)
	}
}

func TestMergeOverrides_InvalidPort(t *testing.T) {
	cfg := &Config{}
	if err := MergeOverrides(cfg, map[string]string{"port": "notanumber"}); err == nil {
		t.Fatal("expected error for invalid port, got nil")
	}
}

func TestMergeOverrides_UnknownKey(t *testing.T) {
	cfg := &Config{}
	if err := MergeOverrides(cfg, map[string]string{"unknown": "value"}); err == nil {
		t.Fatal("expected error for unknown key, got nil")
	}
}

func TestConfig_JSONRoundTrip(t *testing.T) {
	cfg := Config{
		Server: ServerConfig{Host: "127.0.0.1", Port: 9090},
		Defaults: Defaults{
			Scheduler:    "fifo",
			Algorithm:    "strict",
			Unit:         "rps",
			MaxQueueSize: 100,
			Overflow:     "reject",
		},
		Endpoints: []EndpointConfig{
			{Path: "/api", Rate: 10, Unit: "rps", Scheduler: "lifo", Algorithm: "token_bucket",
				MaxQueueSize: 50, Overflow: "block", BurstSize: 5, WindowSeconds: 60},
		},
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Config
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Verify key fields survive the round-trip with snake_case keys.
	if got.Server.Host != cfg.Server.Host || got.Server.Port != cfg.Server.Port {
		t.Errorf("server mismatch: got %+v", got.Server)
	}
	if got.Endpoints[0].MaxQueueSize != 50 {
		t.Errorf("max_queue_size: got %d, want 50", got.Endpoints[0].MaxQueueSize)
	}
	if got.Endpoints[0].BurstSize != 5 {
		t.Errorf("burst_size: got %d, want 5", got.Endpoints[0].BurstSize)
	}

	// Verify snake_case field names.
	var m map[string]interface{}
	json.Unmarshal(data, &m)
	srv := m["server"].(map[string]interface{})
	if _, ok := srv["host"]; !ok {
		t.Error("expected JSON key 'host' in server")
	}
}

// --- InheritFrom tests ---

func TestInheritFrom_EmptyChildInheritsAll(t *testing.T) {
	parent := EndpointConfig{
		Path: "/api", Rate: 10, Unit: "rps", Scheduler: "fifo",
		Algorithm: "token_bucket", MaxQueueSize: 500, Overflow: "block",
		BurstSize: 20, WindowSeconds: 60, QueueTimeout: 3,
	}
	child := EndpointConfig{Path: "/api/v2", Dynamic: true}
	got := InheritFrom(child, parent)

	if got.Path != "/api/v2" {
		t.Errorf("path: got %q, want /api/v2", got.Path)
	}
	if !got.Dynamic {
		t.Error("dynamic should be preserved as true")
	}
	if got.Rate != 10 {
		t.Errorf("rate: got %f, want 10", got.Rate)
	}
	if got.Unit != "rps" {
		t.Errorf("unit: got %q, want rps", got.Unit)
	}
	if got.Scheduler != "fifo" {
		t.Errorf("scheduler: got %q, want fifo", got.Scheduler)
	}
	if got.Algorithm != "token_bucket" {
		t.Errorf("algorithm: got %q, want token_bucket", got.Algorithm)
	}
	if got.MaxQueueSize != 500 {
		t.Errorf("max_queue_size: got %d, want 500", got.MaxQueueSize)
	}
	if got.Overflow != "block" {
		t.Errorf("overflow: got %q, want block", got.Overflow)
	}
	if got.BurstSize != 20 {
		t.Errorf("burst_size: got %d, want 20", got.BurstSize)
	}
	if got.WindowSeconds != 60 {
		t.Errorf("window_seconds: got %d, want 60", got.WindowSeconds)
	}
	if got.QueueTimeout != 3 {
		t.Errorf("queue_timeout: got %f, want 3", got.QueueTimeout)
	}
}

func TestInheritFrom_PartialChildKeepsOwn(t *testing.T) {
	parent := EndpointConfig{
		Rate: 10, Unit: "rps", Scheduler: "fifo", Algorithm: "strict",
		MaxQueueSize: 500, Overflow: "reject", BurstSize: 20, QueueTimeout: 5,
	}
	child := EndpointConfig{
		Path: "/api/v2", Rate: 50, Scheduler: "lifo",
	}
	got := InheritFrom(child, parent)

	if got.Rate != 50 {
		t.Errorf("rate: got %f, want 50 (child's)", got.Rate)
	}
	if got.Scheduler != "lifo" {
		t.Errorf("scheduler: got %q, want lifo (child's)", got.Scheduler)
	}
	if got.Unit != "rps" {
		t.Errorf("unit: got %q, want rps (parent's)", got.Unit)
	}
	if got.Algorithm != "strict" {
		t.Errorf("algorithm: got %q, want strict (parent's)", got.Algorithm)
	}
	if got.MaxQueueSize != 500 {
		t.Errorf("max_queue_size: got %d, want 500 (parent's)", got.MaxQueueSize)
	}
}

func TestInheritFrom_FullySpecifiedIgnoresParent(t *testing.T) {
	parent := EndpointConfig{
		Rate: 10, Unit: "rps", Scheduler: "fifo", Algorithm: "strict",
		MaxQueueSize: 500, Overflow: "reject", BurstSize: 20, WindowSeconds: 30, QueueTimeout: 5,
	}
	child := EndpointConfig{
		Path: "/x", Rate: 99, Unit: "rpm", Scheduler: "lifo", Algorithm: "sliding_window",
		MaxQueueSize: 100, Overflow: "block", BurstSize: 5, WindowSeconds: 10, QueueTimeout: 1,
	}
	got := InheritFrom(child, parent)

	if got.Rate != 99 || got.Unit != "rpm" || got.Scheduler != "lifo" ||
		got.Algorithm != "sliding_window" || got.MaxQueueSize != 100 ||
		got.Overflow != "block" || got.BurstSize != 5 || got.WindowSeconds != 10 ||
		got.QueueTimeout != 1 {
		t.Errorf("fully specified child should ignore parent, got %+v", got)
	}
}

func TestInheritFrom_DynamicAndPathPreserved(t *testing.T) {
	parent := EndpointConfig{Path: "/parent", Dynamic: false, Rate: 10}
	child := EndpointConfig{Path: "/child", Dynamic: true}
	got := InheritFrom(child, parent)

	if got.Path != "/child" {
		t.Errorf("path: got %q, want /child", got.Path)
	}
	if !got.Dynamic {
		t.Error("dynamic should be preserved as true")
	}

	// non-dynamic child
	child2 := EndpointConfig{Path: "/child2", Dynamic: false}
	got2 := InheritFrom(child2, parent)
	if got2.Dynamic {
		t.Error("dynamic=false should be preserved")
	}
}

func TestInheritFrom_LatencyCompensation(t *testing.T) {
	parent := EndpointConfig{Path: "/", Rate: 10, LatencyCompensation: 20}
	child := EndpointConfig{Path: "/child"}
	got := InheritFrom(child, parent)
	if got.LatencyCompensation != 20 {
		t.Errorf("latency_compensation: got %f, want 20", got.LatencyCompensation)
	}

	// Child with own value keeps it.
	child2 := EndpointConfig{Path: "/child2", LatencyCompensation: 5}
	got2 := InheritFrom(child2, parent)
	if got2.LatencyCompensation != 5 {
		t.Errorf("latency_compensation: got %f, want 5", got2.LatencyCompensation)
	}
}

func TestApplyDefaults_LatencyCompensation(t *testing.T) {
	cfg := &Config{
		Defaults: Defaults{LatencyCompensation: 15},
		Endpoints: []EndpointConfig{
			{Path: "/", Rate: 1},
			{Path: "/api", Rate: 2, LatencyCompensation: 10},
		},
	}
	ApplyDefaults(cfg)

	// Root should inherit from defaults.
	for _, ep := range cfg.Endpoints {
		if ep.Path == "/" && ep.LatencyCompensation != 15 {
			t.Errorf("/: latency_compensation: got %f, want 15", ep.LatencyCompensation)
		}
		if ep.Path == "/api" && ep.LatencyCompensation != 10 {
			t.Errorf("/api: latency_compensation: got %f, want 10 (own value)", ep.LatencyCompensation)
		}
	}
}

func TestMergeOverrides_Empty(t *testing.T) {
	cfg := &Config{Server: ServerConfig{Host: "1.2.3.4", Port: 1234}}
	if err := MergeOverrides(cfg, map[string]string{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Host != "1.2.3.4" || cfg.Server.Port != 1234 {
		t.Error("empty overrides should not change config")
	}
}

// --- Token window config tests ---

func TestApplyDefaults_TokenWindowFields(t *testing.T) {
	cfg := &Config{
		Defaults: Defaults{TokensPerWindow: 100, DefaultTokens: 5},
		Endpoints: []EndpointConfig{
			{Path: "/", Rate: 1},
			{Path: "/api", Rate: 1, Algorithm: "token_window", WindowSeconds: 10},
			{Path: "/custom", Rate: 1, Algorithm: "token_window", WindowSeconds: 10, TokensPerWindow: 200, DefaultTokens: 10},
		},
	}
	ApplyDefaults(cfg)

	for _, ep := range cfg.Endpoints {
		switch ep.Path {
		case "/":
			if ep.TokensPerWindow != 100 {
				t.Errorf("/: tokens_per_window: got %d, want 100", ep.TokensPerWindow)
			}
			if ep.DefaultTokens != 5 {
				t.Errorf("/: default_tokens: got %d, want 5", ep.DefaultTokens)
			}
		case "/api":
			if ep.TokensPerWindow != 100 {
				t.Errorf("/api: tokens_per_window: got %d, want 100 (from defaults)", ep.TokensPerWindow)
			}
			if ep.DefaultTokens != 5 {
				t.Errorf("/api: default_tokens: got %d, want 5 (from defaults)", ep.DefaultTokens)
			}
		case "/custom":
			if ep.TokensPerWindow != 200 {
				t.Errorf("/custom: tokens_per_window: got %d, want 200 (own value)", ep.TokensPerWindow)
			}
			if ep.DefaultTokens != 10 {
				t.Errorf("/custom: default_tokens: got %d, want 10 (own value)", ep.DefaultTokens)
			}
		}
	}
}

func TestApplyDefaults_TokenWindow_DefaultTokensFallsTo1(t *testing.T) {
	cfg := &Config{
		Endpoints: []EndpointConfig{
			{Path: "/", Rate: 1},
			{Path: "/api", Rate: 1, Algorithm: "token_window", WindowSeconds: 10, TokensPerWindow: 50},
		},
	}
	ApplyDefaults(cfg)

	for _, ep := range cfg.Endpoints {
		if ep.Path == "/api" && ep.DefaultTokens != 1 {
			t.Errorf("/api: default_tokens: got %d, want 1 (fallback)", ep.DefaultTokens)
		}
	}
}

func TestInheritFrom_TokenWindowFields(t *testing.T) {
	parent := EndpointConfig{
		Path: "/api", Algorithm: "token_window",
		TokensPerWindow: 100, DefaultTokens: 5, WindowSeconds: 30,
	}
	child := EndpointConfig{Path: "/api/sub", Dynamic: true}
	got := InheritFrom(child, parent)

	if got.TokensPerWindow != 100 {
		t.Errorf("tokens_per_window: got %d, want 100", got.TokensPerWindow)
	}
	if got.DefaultTokens != 5 {
		t.Errorf("default_tokens: got %d, want 5", got.DefaultTokens)
	}
	if got.WindowSeconds != 30 {
		t.Errorf("window_seconds: got %d, want 30", got.WindowSeconds)
	}
	if got.Algorithm != "token_window" {
		t.Errorf("algorithm: got %q, want token_window", got.Algorithm)
	}
}

func TestInheritFrom_TokenWindowFields_ChildKeepsOwn(t *testing.T) {
	parent := EndpointConfig{TokensPerWindow: 100, DefaultTokens: 5}
	child := EndpointConfig{Path: "/x", TokensPerWindow: 200, DefaultTokens: 10}
	got := InheritFrom(child, parent)

	if got.TokensPerWindow != 200 {
		t.Errorf("tokens_per_window: got %d, want 200 (child's)", got.TokensPerWindow)
	}
	if got.DefaultTokens != 10 {
		t.Errorf("default_tokens: got %d, want 10 (child's)", got.DefaultTokens)
	}
}

func TestLoad_TokenWindow_Valid(t *testing.T) {
	path := writeTemp(t, `
endpoints:
  - path: "/"
    rate: 1
  - path: "/api"
    algorithm: token_window
    tokens_per_window: 100
    window_seconds: 10
    default_tokens: 5
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, ep := range cfg.Endpoints {
		if ep.Path == "/api" {
			if ep.TokensPerWindow != 100 {
				t.Errorf("tokens_per_window: got %d, want 100", ep.TokensPerWindow)
			}
			if ep.DefaultTokens != 5 {
				t.Errorf("default_tokens: got %d, want 5", ep.DefaultTokens)
			}
		}
	}
}

func TestLoad_TokenWindow_MissingTokensPerWindow(t *testing.T) {
	path := writeTemp(t, `
endpoints:
  - path: "/"
    algorithm: token_window
    window_seconds: 10
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for token_window with tokens_per_window=0, got nil")
	}
}

func TestLoad_TokenWindow_MissingWindowSeconds(t *testing.T) {
	path := writeTemp(t, `
endpoints:
  - path: "/"
    algorithm: token_window
    tokens_per_window: 100
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for token_window with window_seconds=0, got nil")
	}
}
