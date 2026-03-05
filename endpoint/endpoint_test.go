package endpoint

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
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
	if resp.MaxQueueSize != 100 {
		t.Errorf("max_queue_size: got %d, want 100", resp.MaxQueueSize)
	}
	if resp.Overflow != "reject" {
		t.Errorf("overflow: got %q, want reject", resp.Overflow)
	}
	if resp.Dynamic {
		t.Error("dynamic: want false for configured endpoint")
	}
}

func TestBuildResponse_AllConfigFields(t *testing.T) {
	cfg := config.EndpointConfig{
		Path:          "/tb",
		Rate:          10,
		Unit:          "rps",
		Scheduler:     "fifo",
		Algorithm:     "token_bucket",
		MaxQueueSize:  200,
		Overflow:      "block",
		BurstSize:     15,
		WindowSeconds: 30,
		QueueTimeout:  5.5,
		Dynamic:       false,
	}
	resp := buildResponse(cfg, 0, time.Now())

	if resp.Algorithm != "token_bucket" {
		t.Errorf("algorithm: got %q", resp.Algorithm)
	}
	if resp.MaxQueueSize != 200 {
		t.Errorf("max_queue_size: got %d", resp.MaxQueueSize)
	}
	if resp.Overflow != "block" {
		t.Errorf("overflow: got %q", resp.Overflow)
	}
	if resp.BurstSize != 15 {
		t.Errorf("burst_size: got %d", resp.BurstSize)
	}
	if resp.WindowSeconds != 30 {
		t.Errorf("window_seconds: got %d", resp.WindowSeconds)
	}
	if resp.QueueTimeout != 5.5 {
		t.Errorf("queue_timeout: got %f", resp.QueueTimeout)
	}
}

func TestBuildResponse_DynamicEndpoint(t *testing.T) {
	cfg := config.EndpointConfig{
		Path:         "/api/v2/users",
		Rate:         10,
		Unit:         "rps",
		Scheduler:    "fifo",
		Algorithm:    "strict",
		MaxQueueSize: 500,
		Overflow:     "reject",
		Dynamic:      true,
	}
	resp := buildResponse(cfg, 2, time.Now().Add(-100*time.Millisecond))

	if !resp.Dynamic {
		t.Error("dynamic: want true for dynamic endpoint")
	}
	if resp.Endpoint != "/api/v2/users" {
		t.Errorf("endpoint: got %q", resp.Endpoint)
	}
	if resp.Rate != 10 {
		t.Errorf("rate: got %f, want 10 (inherited)", resp.Rate)
	}
	if resp.MaxQueueSize != 500 {
		t.Errorf("max_queue_size: got %d, want 500 (inherited)", resp.MaxQueueSize)
	}
}

func TestBuildResponse_JSONContainsAllFields(t *testing.T) {
	cfg := config.EndpointConfig{
		Path: "/api/v2", Rate: 10, Unit: "rps", Scheduler: "fifo",
		Algorithm: "token_bucket", MaxQueueSize: 500, Overflow: "reject",
		BurstSize: 20, WindowSeconds: 60, QueueTimeout: 3, Dynamic: true,
	}
	resp := buildResponse(cfg, 5, time.Now().Add(-200*time.Millisecond))

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)

	for _, key := range []string{
		`"ok"`, `"endpoint"`, `"queued_for_ms"`, `"queue_depth"`,
		`"rate"`, `"unit"`, `"scheduler"`, `"algorithm"`,
		`"max_queue_size"`, `"overflow"`, `"burst_size"`,
		`"window_seconds"`, `"queue_timeout"`, `"dynamic"`,
	} {
		if !strings.Contains(s, key) {
			t.Errorf("JSON missing key %s in: %s", key, s)
		}
	}
}

func TestBuildResponse_OmitsZeroOptionalFields(t *testing.T) {
	cfg := config.EndpointConfig{
		Path:         "/strict",
		Rate:         1,
		Unit:         "rps",
		Scheduler:    "fifo",
		Algorithm:    "strict",
		MaxQueueSize: 100,
		Overflow:     "reject",
		// BurstSize, WindowSeconds, QueueTimeout all zero
	}
	resp := buildResponse(cfg, 0, time.Now())

	if resp.BurstSize != 0 {
		t.Errorf("burst_size: got %d, want 0", resp.BurstSize)
	}
	if resp.WindowSeconds != 0 {
		t.Errorf("window_seconds: got %d, want 0", resp.WindowSeconds)
	}
	if resp.QueueTimeout != 0 {
		t.Errorf("queue_timeout: got %f, want 0", resp.QueueTimeout)
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
	// Dynamic endpoint creation: path should be the request path with inherited config.
	if ep.cfg.Path != "/api/users" {
		t.Errorf("prefix match returned %q, want /api/users (dynamic)", ep.cfg.Path)
	}
	if !ep.cfg.Dynamic {
		t.Error("expected dynamic endpoint")
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

// --- Admission timeout tests ---

func TestEndpoint_QueueTimeout_RejectsWhenEstimatedWaitExceeds(t *testing.T) {
	cfg := baseConfig("/", 1) // 1 RPS
	cfg.QueueTimeout = 2      // 2s timeout
	cfg.MaxQueueSize = 20

	ep, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer ep.Stop()

	// Fill queue with 3 requests (will block waiting for release at 1 RPS).
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rr := httptest.NewRecorder()
			ep.Handle(rr, req)
		}()
	}
	// Give time for all 3 to be queued.
	time.Sleep(100 * time.Millisecond)

	// 4th request: estimated wait = 3/1 = 3s > 2s timeout → 429
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	ep.Handle(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("status: got %d, want 429", rr.Code)
	}

	ep.Stop()
	wg.Wait()
}

func TestEndpoint_QueueTimeout_AcceptsWithinLimit(t *testing.T) {
	cfg := baseConfig("/", 1)
	cfg.QueueTimeout = 5
	cfg.MaxQueueSize = 20

	ep, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer ep.Stop()

	// Fill queue with 3 (est wait for 4th = 3s < 5s timeout).
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rr := httptest.NewRecorder()
			ep.Handle(rr, req)
		}()
	}
	time.Sleep(100 * time.Millisecond)

	// 4th request: est wait = 3s < 5s → accepted
	done := make(chan int, 1)
	go func() {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		ep.Handle(rr, req)
		done <- rr.Code
	}()

	select {
	case code := <-done:
		if code == http.StatusTooManyRequests {
			t.Error("expected request to be accepted, got 429")
		}
	case <-time.After(10 * time.Second):
		t.Log("request still queued (accepted, not rejected) — OK")
	}

	ep.Stop()
	wg.Wait()
}

func TestEndpoint_QueueTimeout_DisabledByDefault(t *testing.T) {
	cfg := baseConfig("/", 1)
	cfg.QueueTimeout = 0 // disabled
	cfg.MaxQueueSize = 20

	ep, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer ep.Stop()

	// Fill queue with 10 requests.
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rr := httptest.NewRecorder()
			ep.Handle(rr, req)
		}()
	}
	time.Sleep(100 * time.Millisecond)

	// 11th request: no timeout → accepted into queue (not rejected).
	done := make(chan int, 1)
	go func() {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		ep.Handle(rr, req)
		done <- rr.Code
	}()

	select {
	case code := <-done:
		if code == http.StatusTooManyRequests {
			t.Error("expected request to be accepted when timeout=0, got 429")
		}
	case <-time.After(15 * time.Second):
		t.Log("request still queued (accepted) — OK")
	}

	ep.Stop()
	wg.Wait()
}

func TestEndpoint_QueueTimeout_QueryParamOverride(t *testing.T) {
	cfg := baseConfig("/", 1)
	cfg.QueueTimeout = 1 // very tight
	cfg.MaxQueueSize = 20

	ep, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer ep.Stop()

	// Fill queue with 3 (est wait = 3s > 1s config timeout).
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rr := httptest.NewRecorder()
			ep.Handle(rr, req)
		}()
	}
	time.Sleep(100 * time.Millisecond)

	// With ?timeout=999, override allows it through.
	done := make(chan int, 1)
	go func() {
		req := httptest.NewRequest(http.MethodGet, "/?timeout=999", nil)
		rr := httptest.NewRecorder()
		ep.Handle(rr, req)
		done <- rr.Code
	}()

	select {
	case code := <-done:
		if code == http.StatusTooManyRequests {
			t.Error("expected ?timeout=999 to override config, got 429")
		}
	case <-time.After(10 * time.Second):
		t.Log("request still queued (accepted via override) — OK")
	}

	ep.Stop()
	wg.Wait()
}

func TestEndpoint_QueueTimeout_SkippedForLIFO(t *testing.T) {
	cfg := baseConfig("/", 1)
	cfg.Scheduler = "lifo"
	cfg.QueueTimeout = 0.001 // tiny timeout
	cfg.MaxQueueSize = 20

	ep, err := New(cfg)
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
	time.Sleep(100 * time.Millisecond)

	// LIFO: estimateWait returns 0, so timeout check is skipped.
	done := make(chan int, 1)
	go func() {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		ep.Handle(rr, req)
		done <- rr.Code
	}()

	select {
	case code := <-done:
		if code == http.StatusTooManyRequests {
			t.Error("LIFO should skip timeout check, got 429")
		}
	case <-time.After(10 * time.Second):
		t.Log("request still queued (LIFO accepted) — OK")
	}

	ep.Stop()
	wg.Wait()
}

func TestEndpoint_QueueTimeout_SkippedForRandom(t *testing.T) {
	cfg := baseConfig("/", 1)
	cfg.Scheduler = "random"
	cfg.QueueTimeout = 0.001
	cfg.MaxQueueSize = 20

	ep, err := New(cfg)
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
	time.Sleep(100 * time.Millisecond)

	done := make(chan int, 1)
	go func() {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		ep.Handle(rr, req)
		done <- rr.Code
	}()

	select {
	case code := <-done:
		if code == http.StatusTooManyRequests {
			t.Error("random should skip timeout check, got 429")
		}
	case <-time.After(10 * time.Second):
		t.Log("request still queued (random accepted) — OK")
	}

	ep.Stop()
	wg.Wait()
}

func TestEndpoint_QueueTimeout_TokenBucket_BurstAware(t *testing.T) {
	cfg := config.EndpointConfig{
		Path:         "/burst",
		Rate:         1,
		Unit:         "rps",
		Scheduler:    "fifo",
		Algorithm:    "token_bucket",
		BurstSize:    5,
		MaxQueueSize: 20,
		Overflow:     "reject",
		QueueTimeout: 1, // 1s timeout
	}

	ep, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer ep.Stop()

	// Queue 3 requests. With burst=5, available tokens ≥ 3, so ahead = max(0, 3-5) = 0.
	// Estimated wait = 0 < 1s → accepted.
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/burst", nil)
			rr := httptest.NewRecorder()
			ep.Handle(rr, req)
		}()
	}
	time.Sleep(100 * time.Millisecond)

	// 4th: still within burst capacity
	done := make(chan int, 1)
	go func() {
		req := httptest.NewRequest(http.MethodGet, "/burst", nil)
		rr := httptest.NewRecorder()
		ep.Handle(rr, req)
		done <- rr.Code
	}()

	select {
	case code := <-done:
		if code == http.StatusTooManyRequests {
			t.Error("expected token_bucket burst to make request acceptable, got 429")
		}
	case <-time.After(10 * time.Second):
		t.Log("request accepted (burst-aware) — OK")
	}

	ep.Stop()
	wg.Wait()
}

// --- Ghost ticket: client disconnect should not leak handler goroutine ---

func TestEndpoint_ClientDisconnect_HandlerReturns(t *testing.T) {
	cfg := baseConfig("/", 1) // 1 RPS — slow enough that requests queue
	cfg.MaxQueueSize = 20

	ep, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer ep.Stop()

	// Fill the queue so the next request blocks waiting for release.
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rr := httptest.NewRecorder()
			ep.Handle(rr, req)
		}()
	}
	time.Sleep(100 * time.Millisecond)

	// Send a request with a context that we cancel (simulating client disconnect).
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	rr := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		ep.Handle(rr, req)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel() // simulate client disconnect

	select {
	case <-done:
		// Handler returned — no goroutine leak
	case <-time.After(2 * time.Second):
		t.Error("Handle() did not return after client context was cancelled — goroutine leak")
	}

	ep.Stop()
	wg.Wait()
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
	// Dynamic endpoint creation: path should be request path, inheriting from nearest parent /api/v2.
	if ep.cfg.Path != "/api/v2/users" {
		t.Errorf("expected dynamic /api/v2/users, got %q", ep.cfg.Path)
	}
	if !ep.cfg.Dynamic {
		t.Error("expected dynamic endpoint")
	}
}
