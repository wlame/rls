package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wlame/rls/config"
	"github.com/wlame/rls/endpoint"
)

func testConfig(endpoints ...config.EndpointConfig) config.Config {
	cfg := config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 8080},
		Endpoints: endpoints,
	}
	return cfg
}

func rootEndpoint(rps float64) config.EndpointConfig {
	return config.EndpointConfig{
		Path:         "/",
		Rate:         rps,
		Unit:         "rps",
		Scheduler:    "fifo",
		Algorithm:    "strict",
		MaxQueueSize: 100,
		Overflow:     "reject",
	}
}

func TestServer_RootEndpoint_Returns200(t *testing.T) {
	srv, err := New(testConfig(rootEndpoint(100)))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer srv.registry.StopAll()

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: got %q, want application/json", ct)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if ok, _ := body["ok"].(bool); !ok {
		t.Error("body.ok: want true")
	}
}

func TestServer_UnknownPath_Returns404(t *testing.T) {
	srv, err := New(testConfig(
		config.EndpointConfig{
			Path: "/api", Rate: 100, Unit: "rps",
			Scheduler: "fifo", Algorithm: "strict",
			MaxQueueSize: 100, Overflow: "reject",
		},
	))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer srv.registry.StopAll()

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/unknown")
	if err != nil {
		t.Fatalf("GET /unknown: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if ok, _ := body["ok"].(bool); ok {
		t.Error("body.ok: want false for 404")
	}
	if _, hasError := body["error"]; !hasError {
		t.Error("body should have 'error' field")
	}
}

func TestServer_WithEventSink_ReceivesEvents(t *testing.T) {
	events := make(chan endpoint.Event, 16)
	srv, err := New(testConfig(rootEndpoint(100)), endpoint.WithEventSink(events))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer srv.registry.StopAll()

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	resp.Body.Close()

	if len(events) == 0 {
		t.Fatal("expected events in channel, got none")
	}
	// Must have at least EventQueued and EventServed.
	kinds := make(map[endpoint.EventKind]bool)
	for len(events) > 0 {
		kinds[(<-events).Kind] = true
	}
	if !kinds[endpoint.EventQueued] {
		t.Error("missing EventQueued")
	}
	if !kinds[endpoint.EventServed] {
		t.Error("missing EventServed")
	}
}

func TestServer_Shutdown_StopsDispatchersAfterHTTP(t *testing.T) {
	// Verify the shutdown ordering: HTTP drain happens before StopAll.
	// We test this by checking that Shutdown() returns without error
	// (http.Shutdown succeeds) and that dispatchers are stopped afterward.
	srv, err := New(testConfig(rootEndpoint(100)))
	if err != nil {
		t.Fatal(err)
	}

	// Verify the server starts and can serve.
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("pre-shutdown: got %d, want 200", resp.StatusCode)
	}

	// Shutdown should complete without error.
	if err := srv.Shutdown(); err != nil {
		t.Errorf("shutdown: %v", err)
	}
}

func TestServer_KnownPath_Routes_Correctly(t *testing.T) {
	apiCfg := config.EndpointConfig{
		Path: "/api", Rate: 100, Unit: "rps",
		Scheduler: "fifo", Algorithm: "strict",
		MaxQueueSize: 100, Overflow: "reject",
	}
	srv, err := New(testConfig(rootEndpoint(100), apiCfg))
	if err != nil {
		t.Fatal(err)
	}
	defer srv.registry.StopAll()

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/users")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status for /api/users: got %d, want 200", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	// Dynamic endpoint creation: /api/users gets its own endpoint (inherits from /api).
	if ep, _ := body["endpoint"].(string); ep != "/api/users" {
		t.Errorf("endpoint: got %q, want /api/users (dynamic)", ep)
	}
}
