package endpoint

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/wlame/rls/config"
)

func baseConfig(path string, rps float64) config.EndpointConfig {
	return config.EndpointConfig{
		Path:         path,
		Rate:         rps,
		Unit:         "rps",
		Scheduler:    "fifo",
		Algorithm:    "strict",
		MaxQueueSize: 100,
		Overflow:     "reject",
	}
}

// --- Endpoint tests ---

func TestEndpoint_SingleRequest_Returns200(t *testing.T) {
	ep, err := New(baseConfig("/", 100))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer ep.Stop()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	ep.Handle(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", rr.Code)
	}

	var resp Response
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp.OK {
		t.Error("response ok: want true")
	}
	if resp.Endpoint != "/" {
		t.Errorf("endpoint: got %q, want /", resp.Endpoint)
	}
	if resp.Rate != 100 {
		t.Errorf("rate: got %f, want 100", resp.Rate)
	}
	if resp.Scheduler != "fifo" {
		t.Errorf("scheduler: got %q, want fifo", resp.Scheduler)
	}
	if resp.Algorithm != "strict" {
		t.Errorf("algorithm: got %q, want strict", resp.Algorithm)
	}
	if resp.QueuedForMs < 0 {
		t.Errorf("queued_for_ms: got %d, want ≥0", resp.QueuedForMs)
	}
}

func TestEndpoint_ContentTypeJSON(t *testing.T) {
	ep, err := New(baseConfig("/", 100))
	if err != nil {
		t.Fatal(err)
	}
	defer ep.Stop()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	ep.Handle(rr, req)

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type: got %q, want application/json", ct)
	}
}

func TestEndpoint_OverflowReject_Returns429(t *testing.T) {
	cfg := baseConfig("/", 1) // very slow
	cfg.MaxQueueSize = 1      // tiny queue

	ep, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer ep.Stop()

	var mu sync.Mutex
	codes := make([]int, 0)

	// Send 5 concurrent requests; at most 1 fits in queue, rest get 429.
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rr := httptest.NewRecorder()
			ep.Handle(rr, req)
			mu.Lock()
			codes = append(codes, rr.Code)
			mu.Unlock()
		}()
	}
	wg.Wait()

	found429 := false
	for _, code := range codes {
		if code == http.StatusTooManyRequests {
			found429 = true
			break
		}
	}
	if !found429 {
		t.Errorf("expected at least one 429, got codes: %v", codes)
	}
}

func TestEndpoint_PriorityHeader_Parsed(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Priority", "42")
	p := parsePriority(req)
	if p != 42 {
		t.Errorf("priority: got %d, want 42", p)
	}
}

func TestEndpoint_PriorityHeader_Invalid(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Priority", "notanint")
	if p := parsePriority(req); p != 0 {
		t.Errorf("invalid priority should default to 0, got %d", p)
	}
}

// --- Response builder tests ---

func TestBuildResponse_Fields(t *testing.T) {
	cfg := baseConfig("/test", 5)
	now := time.Now().Add(-50 * time.Millisecond) // simulated 50ms wait
	resp := buildResponse(cfg, 3, now)

	if !resp.OK {
		t.Error("ok: want true")
	}
	if resp.Endpoint != "/test" {
		t.Errorf("endpoint: got %q, want /test", resp.Endpoint)
	}
	if resp.QueueDepth != 3 {
		t.Errorf("queue_depth: got %d, want 3", resp.QueueDepth)
	}
	if resp.Rate != 5 {
		t.Errorf("rate: got %f, want 5", resp.Rate)
	}
	if resp.QueuedForMs < 40 || resp.QueuedForMs > 200 {
		t.Errorf("queued_for_ms: got %d, want ~50ms (40–200)", resp.QueuedForMs)
	}
}

// --- Registry tests ---

func TestRegistry_ExactMatch(t *testing.T) {
	cfgs := []config.EndpointConfig{
		baseConfig("/", 100),
		baseConfig("/api", 50),
	}
	reg, err := NewRegistry(cfgs)
	if err != nil {
		t.Fatal(err)
	}
	defer reg.StopAll()

	ep, ok := reg.Match("/api")
	if !ok {
		t.Fatal("expected match for /api")
	}
	if ep.cfg.Path != "/api" {
		t.Errorf("matched wrong endpoint: %q", ep.cfg.Path)
	}
}

func TestRegistry_PrefixMatch(t *testing.T) {
	cfgs := []config.EndpointConfig{
		baseConfig("/api", 50),
	}
	reg, err := NewRegistry(cfgs)
	if err != nil {
		t.Fatal(err)
	}
	defer reg.StopAll()

	ep, ok := reg.Match("/api/users")
	if !ok {
		t.Fatal("expected prefix match for /api/users")
	}
	if ep.cfg.Path != "/api" {
		t.Errorf("prefix match returned %q, want /api", ep.cfg.Path)
	}
}

func TestRegistry_RootFallback(t *testing.T) {
	cfgs := []config.EndpointConfig{
		baseConfig("/", 100),
	}
	reg, err := NewRegistry(cfgs)
	if err != nil {
		t.Fatal(err)
	}
	defer reg.StopAll()

	_, ok := reg.Match("/anything")
	if !ok {
		t.Fatal("expected / to match as fallback for unknown path")
	}
}

func TestRegistry_NoMatch(t *testing.T) {
	cfgs := []config.EndpointConfig{
		baseConfig("/api", 50),
	}
	reg, err := NewRegistry(cfgs)
	if err != nil {
		t.Fatal(err)
	}
	defer reg.StopAll()

	_, ok := reg.Match("/other")
	if ok {
		t.Fatal("expected no match for /other when only /api configured")
	}
}

func TestRegistry_LongestPrefixWins(t *testing.T) {
	cfgs := []config.EndpointConfig{
		baseConfig("/api", 10),
		baseConfig("/api/v2", 20),
	}
	reg, err := NewRegistry(cfgs)
	if err != nil {
		t.Fatal(err)
	}
	defer reg.StopAll()

	ep, ok := reg.Match("/api/v2/users")
	if !ok {
		t.Fatal("expected match")
	}
	if ep.cfg.Path != "/api/v2" {
		t.Errorf("expected longest prefix /api/v2, got %q", ep.cfg.Path)
	}
}
