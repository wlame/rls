package endpoint

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"

	"github.com/wlame/rls/config"
)

func makeRegistryCfgs(paths ...string) []config.EndpointConfig {
	var cfgs []config.EndpointConfig
	for _, p := range paths {
		cfgs = append(cfgs, config.EndpointConfig{
			Path: p, Rate: 5, Unit: "rps", Scheduler: "fifo",
			Algorithm: "strict", MaxQueueSize: 100, Overflow: "reject",
		})
	}
	return cfgs
}

func TestRegistry_Snapshot_ReturnsSorted(t *testing.T) {
	cfgs := makeRegistryCfgs("/", "/z", "/a")
	reg, err := NewRegistryWithOpts(cfgs, []RegistryOption{WithMaxDynamic(10)})
	if err != nil {
		t.Fatal(err)
	}
	defer reg.StopAll()

	snap := reg.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("snapshot len: got %d, want 3", len(snap))
	}
	paths := make([]string, len(snap))
	for i, s := range snap {
		paths[i] = s.Config.Path
	}
	if !sort.StringsAreSorted(paths) {
		t.Errorf("snapshot not sorted: %v", paths)
	}
}

func TestRegistry_Snapshot_ConcurrentAccess(t *testing.T) {
	cfgs := makeRegistryCfgs("/")
	reg, err := NewRegistryWithOpts(cfgs, []RegistryOption{WithMaxDynamic(100)})
	if err != nil {
		t.Fatal(err)
	}
	defer reg.StopAll()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reg.Snapshot()
			reg.Match("/some/path")
		}()
	}
	wg.Wait()
}

func TestRegistry_DynamicCreation_InheritsFromParent(t *testing.T) {
	cfgs := []config.EndpointConfig{
		{Path: "/", Rate: 1, Unit: "rps", Scheduler: "fifo", Algorithm: "strict", MaxQueueSize: 100, Overflow: "reject"},
		{Path: "/api", Rate: 5, Unit: "rps", Scheduler: "fifo", Algorithm: "strict", MaxQueueSize: 200, Overflow: "reject"},
	}
	reg, err := NewRegistryWithOpts(cfgs, []RegistryOption{WithMaxDynamic(10)})
	if err != nil {
		t.Fatal(err)
	}
	defer reg.StopAll()

	ep, ok := reg.Match("/api/v2/users")
	if !ok {
		t.Fatal("expected match")
	}
	if ep.cfg.Path != "/api/v2/users" {
		t.Errorf("path: got %q, want /api/v2/users", ep.cfg.Path)
	}
	if ep.cfg.Rate != 5 {
		t.Errorf("rate: got %f, want 5 (inherited from /api)", ep.cfg.Rate)
	}
	if !ep.cfg.Dynamic {
		t.Error("expected dynamic=true")
	}
}

func TestRegistry_DynamicCreation_InheritsFromRoot(t *testing.T) {
	cfgs := makeRegistryCfgs("/")
	reg, err := NewRegistryWithOpts(cfgs, []RegistryOption{WithMaxDynamic(10)})
	if err != nil {
		t.Fatal(err)
	}
	defer reg.StopAll()

	ep, ok := reg.Match("/foo/bar")
	if !ok {
		t.Fatal("expected match")
	}
	if ep.cfg.Path != "/foo/bar" {
		t.Errorf("path: got %q, want /foo/bar", ep.cfg.Path)
	}
	if ep.cfg.Rate != 5 {
		t.Errorf("rate: got %f, want 5 (inherited from /)", ep.cfg.Rate)
	}
}

func TestRegistry_DynamicCreation_ExactMatchPreferred(t *testing.T) {
	cfgs := makeRegistryCfgs("/", "/api")
	reg, err := NewRegistryWithOpts(cfgs, []RegistryOption{WithMaxDynamic(10)})
	if err != nil {
		t.Fatal(err)
	}
	defer reg.StopAll()

	ep, ok := reg.Match("/api")
	if !ok {
		t.Fatal("expected match")
	}
	if ep.cfg.Dynamic {
		t.Error("exact match should not be dynamic")
	}
}

func TestRegistry_DynamicCreation_SecondRequestReusesDynamic(t *testing.T) {
	cfgs := makeRegistryCfgs("/")
	reg, err := NewRegistryWithOpts(cfgs, []RegistryOption{WithMaxDynamic(10)})
	if err != nil {
		t.Fatal(err)
	}
	defer reg.StopAll()

	ep1, _ := reg.Match("/new/path")
	ep2, _ := reg.Match("/new/path")
	if ep1 != ep2 {
		t.Error("second request should return same endpoint")
	}
}

func TestRegistry_DynamicCreation_ConcurrentSafety(t *testing.T) {
	cfgs := makeRegistryCfgs("/")
	reg, err := NewRegistryWithOpts(cfgs, []RegistryOption{WithMaxDynamic(100)})
	if err != nil {
		t.Fatal(err)
	}
	defer reg.StopAll()

	var wg sync.WaitGroup
	eps := make([]*Endpoint, 50)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ep, ok := reg.Match("/concurrent/path")
			if !ok {
				t.Error("expected match")
			}
			eps[idx] = ep
		}(i)
	}
	wg.Wait()

	for i := 1; i < 50; i++ {
		if eps[i] != eps[0] {
			t.Fatal("all goroutines should get same endpoint")
		}
	}
}

func TestRegistry_DynamicCreation_NearestParentWins(t *testing.T) {
	cfgs := []config.EndpointConfig{
		{Path: "/", Rate: 1, Unit: "rps", Scheduler: "fifo", Algorithm: "strict", MaxQueueSize: 100, Overflow: "reject"},
		{Path: "/a", Rate: 10, Unit: "rps", Scheduler: "fifo", Algorithm: "strict", MaxQueueSize: 100, Overflow: "reject"},
		{Path: "/a/b", Rate: 20, Unit: "rps", Scheduler: "fifo", Algorithm: "strict", MaxQueueSize: 100, Overflow: "reject"},
	}
	reg, err := NewRegistryWithOpts(cfgs, []RegistryOption{WithMaxDynamic(10)})
	if err != nil {
		t.Fatal(err)
	}
	defer reg.StopAll()

	ep, _ := reg.Match("/a/b/c")
	if ep.cfg.Rate != 20 {
		t.Errorf("rate: got %f, want 20 (from /a/b)", ep.cfg.Rate)
	}
}

func TestRegistry_DynamicCreation_MarkedDynamic(t *testing.T) {
	cfgs := makeRegistryCfgs("/")
	reg, err := NewRegistryWithOpts(cfgs, []RegistryOption{WithMaxDynamic(10)})
	if err != nil {
		t.Fatal(err)
	}
	defer reg.StopAll()

	ep, _ := reg.Match("/dynamic/test")
	if !ep.cfg.Dynamic {
		t.Error("dynamic endpoint should have Dynamic=true")
	}
}

func TestRegistry_DynamicCreation_TokenWindowInherited(t *testing.T) {
	cfgs := []config.EndpointConfig{
		{
			Path: "/api", Algorithm: "token_window", Scheduler: "fifo",
			MaxQueueSize: 100, Overflow: "reject",
			TokensPerWindow: 500, WindowSeconds: 10, DefaultTokens: 5,
		},
	}
	reg, err := NewRegistryWithOpts(cfgs, []RegistryOption{WithMaxDynamic(10)})
	if err != nil {
		t.Fatal(err)
	}
	defer reg.StopAll()

	ep, ok := reg.Match("/api/sub")
	if !ok {
		t.Fatal("expected match")
	}
	if ep.cfg.Algorithm != "token_window" {
		t.Errorf("algorithm: got %q, want token_window", ep.cfg.Algorithm)
	}
	if ep.cfg.TokensPerWindow != 500 {
		t.Errorf("tokens_per_window: got %d, want 500", ep.cfg.TokensPerWindow)
	}
	if ep.cfg.DefaultTokens != 5 {
		t.Errorf("default_tokens: got %d, want 5", ep.cfg.DefaultTokens)
	}
	if ep.cfg.WindowSeconds != 10 {
		t.Errorf("window_seconds: got %d, want 10", ep.cfg.WindowSeconds)
	}
	if !ep.cfg.Dynamic {
		t.Error("expected dynamic=true")
	}
	if ep.tokenWindow == nil {
		t.Error("dynamic token_window endpoint should have tokenWindow set")
	}
}

func TestRegistry_DynamicCreation_TokenWindowServesRequests(t *testing.T) {
	cfgs := []config.EndpointConfig{
		{
			Path: "/api", Algorithm: "token_window", Scheduler: "fifo",
			MaxQueueSize: 100, Overflow: "reject",
			TokensPerWindow: 100, WindowSeconds: 60, DefaultTokens: 1,
		},
	}
	reg, err := NewRegistryWithOpts(cfgs, []RegistryOption{WithMaxDynamic(10)})
	if err != nil {
		t.Fatal(err)
	}
	defer reg.StopAll()

	ep, ok := reg.Match("/api/v2")
	if !ok {
		t.Fatal("expected match")
	}

	// Serve a token-window request on the dynamic endpoint.
	req := httptest.NewRequest(http.MethodGet, "/api/v2?tokens=30", nil)
	rr := httptest.NewRecorder()
	ep.Handle(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}

	var resp Response
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp.TokensConsumed == nil || *resp.TokensConsumed != 30 {
		t.Errorf("tokens_consumed: want 30, got %v", resp.TokensConsumed)
	}
	if resp.TokensRemaining == nil || *resp.TokensRemaining != 70 {
		t.Errorf("tokens_remaining: want 70, got %v", resp.TokensRemaining)
	}
}

func TestRegistry_DynamicCreation_CapEnforced(t *testing.T) {
	cfgs := makeRegistryCfgs("/")
	reg, err := NewRegistryWithOpts(cfgs, []RegistryOption{WithMaxDynamic(2)})
	if err != nil {
		t.Fatal(err)
	}
	defer reg.StopAll()

	// Create 2 dynamic endpoints (should succeed)
	reg.Match("/d1")
	reg.Match("/d2")

	// 3rd should fall back to parent
	ep, ok := reg.Match("/d3")
	if !ok {
		t.Fatal("expected match")
	}
	if ep.cfg.Path == "/d3" {
		t.Error("3rd dynamic should fall back to parent, not create new")
	}
	if ep.cfg.Path != "/" {
		t.Errorf("3rd should fall back to /, got %q", ep.cfg.Path)
	}
}
