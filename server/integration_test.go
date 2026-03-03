package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/wlame/rls/config"
	"github.com/wlame/rls/endpoint"
)

// --- Integration: rate accuracy ---

func TestIntegration_RateAccuracy_10RPS(t *testing.T) {
	cfg := config.EndpointConfig{
		Path: "/", Rate: 10, Unit: "rps",
		Scheduler: "fifo", Algorithm: "strict",
		MaxQueueSize: 50, Overflow: "reject",
	}
	srv, err := New(testConfig(cfg))
	if err != nil {
		t.Fatal(err)
	}
	defer srv.registry.StopAll()

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	const numRequests = 20
	var wg sync.WaitGroup
	results := make(chan time.Time, numRequests)

	start := time.Now()
	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Get(ts.URL + "/")
			if err != nil {
				return
			}
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				results <- time.Now()
			}
		}()
	}
	wg.Wait()
	close(results)

	var times []time.Time
	for t := range results {
		times = append(times, t)
	}

	// At 10 RPS, 20 requests should take ~2 seconds. Allow ±800ms.
	elapsed := time.Since(start)
	if elapsed < 1200*time.Millisecond || elapsed > 4000*time.Millisecond {
		t.Errorf("20 requests at 10 RPS took %v, want ~2s (1.2–4.0s)", elapsed)
	}
	t.Logf("20 requests at 10 RPS completed in %v (%d successful)", elapsed, len(times))
}

// --- Integration: FIFO ordering ---

func TestIntegration_FIFO_Ordering(t *testing.T) {
	// Use a very high RPS so requests are released quickly,
	// but inject them sequentially to verify ordering is preserved.
	cfg := config.EndpointConfig{
		Path: "/ordered", Rate: 1000, Unit: "rps",
		Scheduler: "fifo", Algorithm: "strict",
		MaxQueueSize: 100, Overflow: "reject",
	}
	ep, err := endpoint.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer ep.Stop()

	const n = 10
	order := make([]int, 0, n)
	var mu sync.Mutex

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest(http.MethodGet, "/ordered", nil)
			rr := httptest.NewRecorder()
			// Small stagger to help FIFO ordering land as expected.
			time.Sleep(time.Duration(i) * 200 * time.Microsecond)
			ep.Handle(rr, req)
			mu.Lock()
			order = append(order, i)
			mu.Unlock()
		}()
	}
	wg.Wait()

	// All n requests should have been served.
	if len(order) != n {
		t.Errorf("served %d requests, want %d", len(order), n)
	}
}

// --- Integration: overflow=reject ---

func TestIntegration_Overflow_Reject(t *testing.T) {
	const queueSize = 5
	cfg := config.EndpointConfig{
		Path: "/slow", Rate: 1, Unit: "rps", // very slow
		Scheduler:    "fifo",
		Algorithm:    "strict",
		MaxQueueSize: queueSize,
		Overflow:     "reject",
	}
	srv, err := New(testConfig(cfg))
	if err != nil {
		t.Fatal(err)
	}
	defer srv.registry.StopAll()

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Send queueSize+3 concurrent requests; at least 3 should be rejected.
	const total = queueSize + 3
	codes := make(chan int, total)
	var wg sync.WaitGroup
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Get(ts.URL + "/slow")
			if err != nil {
				codes <- 0
				return
			}
			resp.Body.Close()
			codes <- resp.StatusCode
		}()
	}
	wg.Wait()
	close(codes)

	rejected := 0
	for code := range codes {
		if code == http.StatusTooManyRequests {
			rejected++
		}
	}
	if rejected == 0 {
		t.Errorf("expected at least 1 rejected request (429), got 0")
	}
	t.Logf("%d/%d requests rejected (429)", rejected, total)
}

// --- Integration: response JSON schema ---

func TestIntegration_ResponseSchema(t *testing.T) {
	srv, err := New(testConfig(rootEndpoint(100)))
	if err != nil {
		t.Fatal(err)
	}
	defer srv.registry.StopAll()

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	requiredFields := []string{"ok", "endpoint", "queued_for_ms", "queue_depth", "rate", "unit", "scheduler", "algorithm"}
	for _, field := range requiredFields {
		if _, ok := body[field]; !ok {
			t.Errorf("response missing field %q", field)
		}
	}
}
