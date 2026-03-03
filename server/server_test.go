package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wlame/rls/config"
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
	if ep, _ := body["endpoint"].(string); ep != "/api" {
		t.Errorf("endpoint: got %q, want /api", ep)
	}
}
