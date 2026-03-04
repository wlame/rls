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
	return NewModel(testConfig(), ch, DefaultDotThresholds(), nil, 0)
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

func TestUpdate_UnknownPath_NoChange(t *testing.T) {
	m := newTestModel()
	ev := endpoint.Event{Kind: endpoint.EventQueued, Path: "/nonexistent"}
	updated, _ := m.Update(serverEventMsg{ev: ev})
	m2 := updated.(Model)
	for _, st := range m2.endpoints {
		if len(st.enqueuedAt) != 0 {
			t.Error("unknown path should not affect any endpoint state")
		}
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
	if !strings.Contains(view, "/fast") {
		t.Error("view should contain /fast")
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
	m := NewModel(testConfig(), ch, DefaultDotThresholds(), nil, 12345)
	m.width = 100
	m.height = 20
	view := m.View()
	if !strings.Contains(view, "PID 12345") {
		t.Error("view should contain PID 12345 when attached")
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
