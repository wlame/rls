package endpoint

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
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

// --- Token bucket burst regression ---

// TestEndpoint_TokenBucket_BurstPreservedWhenQueueIdle is a regression test for
// the bug where dispatch() consumed burst tokens from the limiter while the queue
// was empty, so all tokens were wasted before any request arrived.
//
// Setup: burst=5, rate=1 RPS. After a 200ms idle pause (long enough for the old
// buggy code to drain all pre-filled tokens), fire 7 concurrent requests.
// The first 5 must complete quickly (burst), the remaining 2 must each wait ~1s.
func TestEndpoint_TokenBucket_BurstPreservedWhenQueueIdle(t *testing.T) {
	const burst = 5
	cfg := config.EndpointConfig{
		Path:         "/burst",
		Rate:         1, // slow post-burst rate makes throttling easy to detect
		Unit:         "rps",
		Scheduler:    "fifo",
		Algorithm:    "token_bucket",
		BurstSize:    burst,
		MaxQueueSize: 20,
		Overflow:     "reject",
	}
	ep, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer ep.Stop()

	// Idle pause: in the old buggy code, dispatch() would drain all burst tokens
	// here because it consumed ticks even when the queue was empty.
	// In the fixed code, burst tokens stay available until requests arrive.
	time.Sleep(200 * time.Millisecond)

	const total = burst + 2
	type result struct{ elapsed time.Duration }
	results := make(chan result, total)

	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/burst", nil)
			rr := httptest.NewRecorder()
			ep.Handle(rr, req)
			results <- result{elapsed: time.Since(start)}
		}()
	}
	wg.Wait()
	close(results)

	elapseds := make([]time.Duration, 0, total)
	for r := range results {
		elapseds = append(elapseds, r.elapsed)
	}
	sort.Slice(elapseds, func(i, j int) bool { return elapseds[i] < elapseds[j] })

	// First 'burst' responses must arrive well within one rate interval (1s).
	for i := 0; i < burst; i++ {
		if elapseds[i] >= 500*time.Millisecond {
			t.Errorf("burst request %d: took %v, want <500ms (burst tokens wasted)", i+1, elapseds[i])
		}
	}
	// Remaining responses must wait significantly longer than burst requests.
	// With a 200ms idle head-start, the first throttled token arrives at ~800ms,
	// so we use 700ms as the lower bound (leaves 100ms margin for timing variance).
	for i := burst; i < total; i++ {
		if elapseds[i] < 700*time.Millisecond {
			t.Errorf("throttled request %d: took %v, want ≥700ms", i+1, elapseds[i])
		}
	}
	t.Logf("burst=%v throttled=%v", elapseds[:burst], elapseds[burst:])
}

// --- Event emission tests ---

func TestEndpoint_EmitQueued_OnSuccessfulPush(t *testing.T) {
	ch := make(chan Event, 10)
	ep, err := New(baseConfig("/", 100), WithEventSink(ch))
	if err != nil {
		t.Fatal(err)
	}
	defer ep.Stop()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	ep.Handle(rr, req)

	// Should have received EventQueued then EventServed.
	if len(ch) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(ch))
	}
	queued := <-ch
	if queued.Kind != EventQueued {
		t.Errorf("first event: want EventQueued, got %v", queued.Kind)
	}
	if queued.Path != "/" {
		t.Errorf("queued path: want /, got %q", queued.Path)
	}
}

func TestEndpoint_EmitServed_AfterRelease(t *testing.T) {
	ch := make(chan Event, 10)
	ep, err := New(baseConfig("/", 100), WithEventSink(ch))
	if err != nil {
		t.Fatal(err)
	}
	defer ep.Stop()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	ep.Handle(rr, req)

	// Drain EventQueued.
	<-ch
	// Next must be EventServed.
	served := <-ch
	if served.Kind != EventServed {
		t.Errorf("second event: want EventServed, got %v", served.Kind)
	}
	if served.Path != "/" {
		t.Errorf("served path: want /, got %q", served.Path)
	}
	if served.WaitedMs < 0 {
		t.Errorf("served WaitedMs: want ≥0, got %d", served.WaitedMs)
	}
}

func TestEndpoint_EmitRejected_OnQueueFull(t *testing.T) {
	cfg := baseConfig("/", 1)
	cfg.MaxQueueSize = 1

	ch := make(chan Event, 20)
	ep, err := New(cfg, WithEventSink(ch))
	if err != nil {
		t.Fatal(err)
	}
	defer ep.Stop()

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rr := httptest.NewRecorder()
			ep.Handle(rr, req)
		}()
	}
	wg.Wait()

	found := false
	for {
		select {
		case ev := <-ch:
			if ev.Kind == EventRejected {
				found = true
			}
		default:
			goto done
		}
	}
done:
	if !found {
		t.Error("expected at least one EventRejected, got none")
	}
}

func TestEndpoint_Emit_DropsWhenFull(t *testing.T) {
	// A channel of size 1 that is already full must not deadlock emit().
	ch := make(chan Event, 1)
	ep := &Endpoint{events: ch}
	ch <- Event{} // fill it

	done := make(chan struct{})
	go func() {
		ep.emit(Event{Kind: EventQueued, Path: "/test"})
		close(done)
	}()

	select {
	case <-done:
		// good — emit returned without blocking
	case <-time.After(100 * time.Millisecond):
		t.Error("emit blocked on full channel")
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
