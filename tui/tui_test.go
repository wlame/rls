package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/wlame/rls/config"
	"github.com/wlame/rls/endpoint"
)

// --- helper ---

func testConfig() *config.Config {
	return &config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 8080},
		Endpoints: []config.EndpointConfig{
			{Path: "/", Rate: 1, Unit: "rps", Scheduler: "fifo", Algorithm: "strict", MaxQueueSize: 10},
			{Path: "/fast", Rate: 5, Unit: "rps", Scheduler: "fifo", Algorithm: "strict", MaxQueueSize: 20},
		},
	}
}

func newTestModel() Model {
	ch := make(chan endpoint.Event, 16)
	return NewModel(testConfig(), ch, DefaultDotThresholds(), nil, 0, nil)
}

// --- computePercentiles ---

func TestComputePercentiles_Empty(t *testing.T) {
	p50, p90, p99 := computePercentiles(nil)
	if p50 != 0 || p90 != 0 || p99 != 0 {
		t.Errorf("empty: want 0,0,0 got %d,%d,%d", p50, p90, p99)
	}
}

func TestComputePercentiles_Single(t *testing.T) {
	p50, p90, p99 := computePercentiles([]int64{42})
	if p50 != 42 || p90 != 42 || p99 != 42 {
		t.Errorf("single: want 42,42,42 got %d,%d,%d", p50, p90, p99)
	}
}

func TestComputePercentiles_Many(t *testing.T) {
	// 100 samples 1..100; p50=50, p90=90, p99=99
	samples := make([]int64, 100)
	for i := range samples {
		samples[i] = int64(i + 1)
	}
	p50, p90, p99 := computePercentiles(samples)
	if p50 != 50 {
		t.Errorf("p50: want 50, got %d", p50)
	}
	if p90 != 90 {
		t.Errorf("p90: want 90, got %d", p90)
	}
	if p99 != 99 {
		t.Errorf("p99: want 99, got %d", p99)
	}
}

func TestComputePercentiles_DoesNotMutateInput(t *testing.T) {
	samples := []int64{5, 3, 1, 4, 2}
	orig := make([]int64, len(samples))
	copy(orig, samples)
	computePercentiles(samples)
	for i, v := range samples {
		if v != orig[i] {
			t.Errorf("input mutated at index %d: %d → %d", i, orig[i], v)
		}
	}
}

// --- appendSample ---

func TestAppendSample_CapsAt200(t *testing.T) {
	var s []int64
	for i := 0; i < 250; i++ {
		s = appendSample(s, int64(i))
	}
	if len(s) != 200 {
		t.Errorf("len: want 200, got %d", len(s))
	}
	// Should have kept the most recent 200 (50..249).
	if s[0] != 50 {
		t.Errorf("first element after cap: want 50, got %d", s[0])
	}
}

// --- Update: EventQueued adds enqueuedAt entry ---

func TestUpdate_EventQueued_AddsEntry(t *testing.T) {
	m := newTestModel()
	before := len(m.endpoints[0].enqueuedAt)

	ev := endpoint.Event{Kind: endpoint.EventQueued, Path: "/"}
	updated, _ := m.Update(serverEventMsg{ev: ev})
	m2 := updated.(Model)

	if len(m2.endpoints[0].enqueuedAt) != before+1 {
		t.Errorf("enqueuedAt: want %d, got %d", before+1, len(m2.endpoints[0].enqueuedAt))
	}
}

// --- Update: EventServed removes entry and increments served ---

func TestUpdate_EventServed_RemovesEntryAndIncrementsServed(t *testing.T) {
	m := newTestModel()
	// Pre-populate one enqueued entry.
	m.endpoints[0].enqueuedAt = []time.Time{time.Now()}
	m.endpoints[0].served = 0

	ev := endpoint.Event{Kind: endpoint.EventServed, Path: "/", WaitedMs: 42, QueueDepth: 0}
	updated, _ := m.Update(serverEventMsg{ev: ev})
	m2 := updated.(Model)

	if len(m2.endpoints[0].enqueuedAt) != 0 {
		t.Errorf("enqueuedAt: want 0, got %d", len(m2.endpoints[0].enqueuedAt))
	}
	if m2.endpoints[0].served != 1 {
		t.Errorf("served: want 1, got %d", m2.endpoints[0].served)
	}
	if m2.endpoints[0].lastWaitMs != 42 {
		t.Errorf("lastWaitMs: want 42, got %d", m2.endpoints[0].lastWaitMs)
	}
}

// --- Update: EventRejected increments rejected ---

func TestUpdate_EventRejected_IncrementsRejected(t *testing.T) {
	m := newTestModel()
	ev := endpoint.Event{Kind: endpoint.EventRejected, Path: "/fast"}
	updated, _ := m.Update(serverEventMsg{ev: ev})
	m2 := updated.(Model)
	if m2.endpoints[1].rejected != 1 {
		t.Errorf("rejected: want 1, got %d", m2.endpoints[1].rejected)
	}
}

// --- Update: unknown path is ignored safely ---

func TestUpdate_UnknownPath_CreatesDynamicEndpoint(t *testing.T) {
	m := newTestModel()
	initialCount := len(m.endpoints)
	ev := endpoint.Event{Kind: endpoint.EventQueued, Path: "/nonexistent"}
	updated, _ := m.Update(serverEventMsg{ev: ev})
	m2 := updated.(Model)

	// Original endpoints should be unaffected.
	for _, st := range m2.endpoints {
		if st.cfg.Path != "/nonexistent" && len(st.enqueuedAt) != 0 {
			t.Error("unknown path event should not affect existing endpoints")
		}
	}
	// A new dynamic endpoint should have been created.
	if len(m2.endpoints) != initialCount+1 {
		t.Errorf("expected %d endpoints (new dynamic), got %d", initialCount+1, len(m2.endpoints))
	}
	idx := m2.indexForPath("/nonexistent")
	if idx < 0 {
		t.Fatal("expected /nonexistent to be created as dynamic endpoint")
	}
	if !m2.endpoints[idx].dynamic {
		t.Error("/nonexistent should be marked dynamic")
	}
}

// --- Key handling ---

func TestUpdate_KeyDown_MovesSelection(t *testing.T) {
	m := newTestModel()
	m.selected = 0
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m2 := updated.(Model)
	if m2.selected != 1 {
		t.Errorf("selected after down: want 1, got %d", m2.selected)
	}
}

func TestUpdate_KeyDown_ClampedAtEnd(t *testing.T) {
	m := newTestModel()
	m.selected = len(m.endpoints) - 1
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m2 := updated.(Model)
	if m2.selected != len(m.endpoints)-1 {
		t.Errorf("selected should not exceed last index, got %d", m2.selected)
	}
}

func TestUpdate_KeyUp_MovesSelection(t *testing.T) {
	m := newTestModel()
	m.selected = 1
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m2 := updated.(Model)
	if m2.selected != 0 {
		t.Errorf("selected after up: want 0, got %d", m2.selected)
	}
}

func TestUpdate_KeyP_TogglesePause(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m2 := updated.(Model)
	if !m2.paused {
		t.Error("paused: want true after p")
	}
	updated, _ = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m3 := updated.(Model)
	if m3.paused {
		t.Error("paused: want false after second p")
	}
}

func TestUpdate_KeyR_ResetsSelectedStats(t *testing.T) {
	m := newTestModel()
	m.endpoints[0].served = 99
	m.endpoints[0].rejected = 5
	m.endpoints[0].waitSamples = []int64{1, 2, 3}
	m.selected = 0

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	m2 := updated.(Model)
	st := m2.endpoints[0]
	if st.served != 0 || st.rejected != 0 || len(st.waitSamples) != 0 {
		t.Errorf("reset failed: served=%d rejected=%d samples=%d", st.served, st.rejected, len(st.waitSamples))
	}
}

func TestUpdate_WindowSize_UpdatesDimensions(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m2 := updated.(Model)
	if m2.width != 120 || m2.height != 40 {
		t.Errorf("dimensions: want 120×40, got %d×%d", m2.width, m2.height)
	}
}

// --- View ---

func TestView_ContainsEndpointPaths(t *testing.T) {
	m := newTestModel()
	m.width = 100
	m.height = 20
	view := m.View()
	if !strings.Contains(view, "/") {
		t.Error("view should contain /")
	}
	// Tree rendering shows "fast" (stripped prefix from /fast under /)
	if !strings.Contains(view, "fast") {
		t.Error("view should contain fast")
	}
}

func TestView_ContainsServerAddr(t *testing.T) {
	m := newTestModel()
	m.width = 100
	m.height = 20
	view := m.View()
	if !strings.Contains(view, "127.0.0.1") {
		t.Error("view should contain server address")
	}
}

func TestView_ContainsHelpBar(t *testing.T) {
	m := newTestModel()
	m.width = 100
	m.height = 20
	view := m.View()
	if !strings.Contains(view, "quit") {
		t.Error("view should contain quit hint")
	}
}

func TestView_ShowsPausedWhenPaused(t *testing.T) {
	m := newTestModel()
	m.width = 100
	m.height = 20
	m.paused = true
	view := m.View()
	if !strings.Contains(view, "PAUSED") {
		t.Error("view should show PAUSED indicator when paused")
	}
}

func TestView_AttachedPID(t *testing.T) {
	ch := make(chan endpoint.Event, 16)
	m := NewModel(testConfig(), ch, DefaultDotThresholds(), nil, 12345, nil)
	m.width = 100
	m.height = 20
	view := m.View()
	if !strings.Contains(view, "PID 12345") {
		t.Error("view should contain PID 12345 when attached")
	}
}

// --- buildTreeLabels ---

func TestBuildTreeLabels_FlatPaths(t *testing.T) {
	// All top-level paths are children of /, so depth=1 (except / itself at depth=0)
	eps := []endpointState{
		{cfg: config.EndpointConfig{Path: "/"}},
		{cfg: config.EndpointConfig{Path: "/api"}},
		{cfg: config.EndpointConfig{Path: "/timeout"}},
	}
	labels := buildTreeLabels(eps)
	if labels[0].depth != 0 || labels[0].label != "/" {
		t.Errorf("root: depth=%d label=%q", labels[0].depth, labels[0].label)
	}
	if labels[1].depth != 1 || labels[1].label != "api" {
		t.Errorf("/api: depth=%d label=%q, want depth=1 label=api", labels[1].depth, labels[1].label)
	}
	if labels[2].depth != 1 || labels[2].label != "timeout" {
		t.Errorf("/timeout: depth=%d label=%q, want depth=1 label=timeout", labels[2].depth, labels[2].label)
	}
}

func TestBuildTreeLabels_NestedPaths(t *testing.T) {
	eps := []endpointState{
		{cfg: config.EndpointConfig{Path: "/"}},
		{cfg: config.EndpointConfig{Path: "/api"}},
		{cfg: config.EndpointConfig{Path: "/api/v2"}},
	}
	labels := buildTreeLabels(eps)
	if labels[0].depth != 0 || labels[0].label != "/" {
		t.Errorf("root: depth=%d label=%q", labels[0].depth, labels[0].label)
	}
	if labels[1].depth != 1 || labels[1].label != "api" {
		t.Errorf("/api: depth=%d label=%q, want depth=1 label=api", labels[1].depth, labels[1].label)
	}
	if labels[2].depth != 2 || labels[2].label != "v2" {
		t.Errorf("/api/v2: depth=%d label=%q, want depth=2 label=v2", labels[2].depth, labels[2].label)
	}
}

func TestBuildTreeLabels_MaxDepth3_Flattens(t *testing.T) {
	eps := []endpointState{
		{cfg: config.EndpointConfig{Path: "/"}},
		{cfg: config.EndpointConfig{Path: "/a"}},
		{cfg: config.EndpointConfig{Path: "/a/b"}},
		{cfg: config.EndpointConfig{Path: "/a/b/c"}},
		{cfg: config.EndpointConfig{Path: "/a/b/c/d"}},
	}
	labels := buildTreeLabels(eps)
	// /a/b/c/d has depth 4 from root → capped at 3
	if labels[4].depth != 3 {
		t.Errorf("/a/b/c/d: depth want 3, got %d", labels[4].depth)
	}
}

func TestBuildTreeLabels_OnlyRegisteredParents(t *testing.T) {
	eps := []endpointState{
		{cfg: config.EndpointConfig{Path: "/"}},
		{cfg: config.EndpointConfig{Path: "/a/b/c"}},
	}
	labels := buildTreeLabels(eps)
	// /a and /a/b are not registered, so /a/b/c should be child of / at depth 1
	if labels[1].depth != 1 {
		t.Errorf("/a/b/c: depth want 1, got %d", labels[1].depth)
	}
	if labels[1].label != "a/b/c" {
		t.Errorf("/a/b/c: label want a/b/c, got %q", labels[1].label)
	}
}

func TestBuildTreeLabels_DynamicFlag(t *testing.T) {
	eps := []endpointState{
		{cfg: config.EndpointConfig{Path: "/"}, dynamic: false},
		{cfg: config.EndpointConfig{Path: "/api"}, dynamic: true},
	}
	labels := buildTreeLabels(eps)
	if labels[0].dynamic {
		t.Error("/ should not be dynamic")
	}
	if !labels[1].dynamic {
		t.Error("/api should be dynamic")
	}
}

func TestSyncEndpoints_AddsNewDynamicEndpoint(t *testing.T) {
	cfgs := []config.EndpointConfig{
		{Path: "/", Rate: 1, Unit: "rps", Scheduler: "fifo", Algorithm: "strict", MaxQueueSize: 100, Overflow: "reject"},
	}
	reg, err := endpoint.NewRegistry(cfgs)
	if err != nil {
		t.Fatal(err)
	}
	defer reg.StopAll()

	ch := make(chan endpoint.Event, 16)
	cfg := &config.Config{
		Server:    config.ServerConfig{Host: "127.0.0.1", Port: 8080},
		Endpoints: cfgs,
	}
	m := NewModel(cfg, ch, DefaultDotThresholds(), nil, 0, nil)
	m.registry = reg

	if len(m.endpoints) != 1 {
		t.Fatalf("initial endpoints: want 1, got %d", len(m.endpoints))
	}

	// Trigger dynamic endpoint creation via registry.
	reg.Match("/api")

	m.syncEndpoints()
	if len(m.endpoints) != 2 {
		t.Fatalf("after sync: want 2, got %d", len(m.endpoints))
	}
}

func TestSyncEndpoints_PreservesExistingStats(t *testing.T) {
	cfgs := []config.EndpointConfig{
		{Path: "/", Rate: 1, Unit: "rps", Scheduler: "fifo", Algorithm: "strict", MaxQueueSize: 100, Overflow: "reject"},
	}
	reg, err := endpoint.NewRegistry(cfgs)
	if err != nil {
		t.Fatal(err)
	}
	defer reg.StopAll()

	ch := make(chan endpoint.Event, 16)
	cfg := &config.Config{
		Server:    config.ServerConfig{Host: "127.0.0.1", Port: 8080},
		Endpoints: cfgs,
	}
	m := NewModel(cfg, ch, DefaultDotThresholds(), nil, 0, nil)
	m.registry = reg
	m.endpoints[0].served = 42
	m.endpoints[0].rejected = 5

	m.syncEndpoints()
	if m.endpoints[0].served != 42 {
		t.Errorf("served: want 42, got %d", m.endpoints[0].served)
	}
	if m.endpoints[0].rejected != 5 {
		t.Errorf("rejected: want 5, got %d", m.endpoints[0].rejected)
	}
}

func TestView_TreeRendering_ShowsIndentedEndpoints(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 8080},
		Endpoints: []config.EndpointConfig{
			{Path: "/", Rate: 1, Unit: "rps", Scheduler: "fifo", Algorithm: "strict", MaxQueueSize: 10},
			{Path: "/api", Rate: 5, Unit: "rps", Scheduler: "fifo", Algorithm: "strict", MaxQueueSize: 10},
			{Path: "/api/v2", Rate: 5, Unit: "rps", Scheduler: "fifo", Algorithm: "strict", MaxQueueSize: 10, Dynamic: true},
		},
	}
	ch := make(chan endpoint.Event, 16)
	m := NewModel(cfg, ch, DefaultDotThresholds(), nil, 0, nil)
	m.endpoints[2].dynamic = true
	m.treeLabels = buildTreeLabels(m.endpoints)
	m.width = 120
	m.height = 20
	view := m.View()
	if !strings.Contains(view, "└") {
		t.Error("view should contain tree connector └")
	}
}

func TestView_DynamicEndpoint_Dim(t *testing.T) {
	// Dynamic endpoint should use dynamicRowStyle (not bold).
	cfg := &config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 8080},
		Endpoints: []config.EndpointConfig{
			{Path: "/", Rate: 1, Unit: "rps", Scheduler: "fifo", Algorithm: "strict", MaxQueueSize: 10},
			{Path: "/dyn", Rate: 1, Unit: "rps", Scheduler: "fifo", Algorithm: "strict", MaxQueueSize: 10},
		},
	}
	ch := make(chan endpoint.Event, 16)
	m := NewModel(cfg, ch, DefaultDotThresholds(), nil, 0, nil)
	m.endpoints[1].dynamic = true
	m.treeLabels = buildTreeLabels(m.endpoints)
	m.width = 100
	m.height = 20
	// Just verify it renders without panic.
	view := m.View()
	if view == "" {
		t.Error("view should not be empty")
	}
}

// --- EventServed dot removal for random scheduler ---

func TestUpdate_EventServed_RandomScheduler_RemovesRandomDot(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 8080},
		Endpoints: []config.EndpointConfig{
			{Path: "/rand", Rate: 1, Unit: "rps", Scheduler: "random", Algorithm: "strict", MaxQueueSize: 10},
		},
	}
	ch := make(chan endpoint.Event, 16)
	m := NewModel(cfg, ch, DefaultDotThresholds(), nil, 0, nil)

	// Seed 3 dots with distinct timestamps.
	t1 := time.Now().Add(-3 * time.Second)
	t2 := time.Now().Add(-2 * time.Second)
	t3 := time.Now().Add(-1 * time.Second)
	m.endpoints[0].enqueuedAt = []time.Time{t1, t2, t3}

	// After serving, should have 2 dots remaining.
	ev := endpoint.Event{Kind: endpoint.EventServed, Path: "/rand", WaitedMs: 10, QueueDepth: 2}
	updated, _ := m.Update(serverEventMsg{ev: ev})
	m2 := updated.(Model)

	if len(m2.endpoints[0].enqueuedAt) != 2 {
		t.Fatalf("random serve: want 2 remaining dots, got %d", len(m2.endpoints[0].enqueuedAt))
	}

	// For random scheduler, it should NOT always remove the oldest (index 0).
	// Run many times and check that sometimes the oldest survives.
	// If always removing index 0, t1 would never survive.
	survivedOldest := false
	for i := 0; i < 100; i++ {
		m3 := NewModel(cfg, ch, DefaultDotThresholds(), nil, 0, nil)
		m3.endpoints[0].enqueuedAt = []time.Time{t1, t2, t3}
		u, _ := m3.Update(serverEventMsg{ev: ev})
		m4 := u.(Model)
		for _, ts := range m4.endpoints[0].enqueuedAt {
			if ts.Equal(t1) {
				survivedOldest = true
				break
			}
		}
		if survivedOldest {
			break
		}
	}
	if !survivedOldest {
		t.Error("random scheduler should sometimes preserve the oldest dot, but it was always removed (index 0 bias)")
	}
}

// --- Attach mode: dynamic endpoint discovery from events ---

func TestHandleServerEvent_UnknownPath_CreatesDynamicEndpoint(t *testing.T) {
	// Simulate attach mode: TUI has only "/" from config, no registry.
	m := newTestModel()
	m.registry = nil // attach mode — no registry

	// Event for a path that doesn't exist in m.endpoints.
	ev := endpoint.Event{Kind: endpoint.EventQueued, Path: "/api/v2/users"}
	updated, _ := m.Update(serverEventMsg{ev: ev})
	m2 := updated.(Model)

	// Should have created a new dynamic endpoint entry.
	found := false
	for _, st := range m2.endpoints {
		if st.cfg.Path == "/api/v2/users" {
			found = true
			if !st.dynamic {
				t.Error("/api/v2/users should be marked dynamic")
			}
			if len(st.enqueuedAt) != 1 {
				t.Errorf("should have 1 enqueued dot, got %d", len(st.enqueuedAt))
			}
			break
		}
	}
	if !found {
		t.Error("expected dynamic endpoint /api/v2/users to be created from event")
	}
}

func TestHandleServerEvent_DynamicEndpoint_InheritsFromParent(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 8080},
		Endpoints: []config.EndpointConfig{
			{Path: "/", Rate: 1, Unit: "rps", Scheduler: "fifo", Algorithm: "strict", MaxQueueSize: 10, Overflow: "reject"},
			{Path: "/api", Rate: 10, Unit: "rps", Scheduler: "priority", Algorithm: "token_bucket", MaxQueueSize: 500, Overflow: "reject", BurstSize: 20},
		},
	}
	ch := make(chan endpoint.Event, 16)
	m := NewModel(cfg, ch, DefaultDotThresholds(), nil, 42, nil) // attach mode (PID != 0, no registry)

	ev := endpoint.Event{Kind: endpoint.EventQueued, Path: "/api/v2/users"}
	updated, _ := m.Update(serverEventMsg{ev: ev})
	m2 := updated.(Model)

	idx := m2.indexForPath("/api/v2/users")
	if idx < 0 {
		t.Fatal("expected /api/v2/users to exist")
	}
	st := m2.endpoints[idx]
	// Should inherit from /api (nearest parent).
	if st.cfg.Rate != 10 {
		t.Errorf("rate: want 10 (inherited from /api), got %.0f", st.cfg.Rate)
	}
	if st.cfg.Scheduler != "priority" {
		t.Errorf("scheduler: want priority (inherited), got %s", st.cfg.Scheduler)
	}
	if st.cfg.MaxQueueSize != 500 {
		t.Errorf("max_queue_size: want 500 (inherited), got %d", st.cfg.MaxQueueSize)
	}
}

func TestHandleServerEvent_DynamicEndpoint_TreeLabelsUpdated(t *testing.T) {
	m := newTestModel()
	m.registry = nil

	ev := endpoint.Event{Kind: endpoint.EventQueued, Path: "/api"}
	updated, _ := m.Update(serverEventMsg{ev: ev})
	m2 := updated.(Model)

	if len(m2.treeLabels) != len(m2.endpoints) {
		t.Errorf("treeLabels length %d != endpoints length %d", len(m2.treeLabels), len(m2.endpoints))
	}
}

func TestView_QueueCounterPresent(t *testing.T) {
	m := newTestModel()
	m.width = 100
	m.height = 20
	// Pre-populate 3 queued requests on first endpoint.
	m.endpoints[0].enqueuedAt = []time.Time{time.Now(), time.Now(), time.Now()}
	view := m.View()
	if !strings.Contains(view, "[3/10]") {
		t.Errorf("view should show [3/10] counter, got:\n%s", view)
	}
}
