package endpoint

import (
	"encoding/json"
	"testing"
)

func TestEvent_JSONRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		ev   Event
	}{
		{"queued", Event{Kind: EventQueued, Path: "/api", Priority: 5}},
		{"served", Event{Kind: EventServed, Path: "/fast", WaitedMs: 42, QueueDepth: 3}},
		{"rejected", Event{Kind: EventRejected, Path: "/"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.ev)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var got Event
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got != tc.ev {
				t.Errorf("round-trip mismatch: got %+v, want %+v", got, tc.ev)
			}
		})
	}
}

func TestEvent_JSONFieldNames(t *testing.T) {
	ev := Event{Kind: EventServed, Path: "/test", WaitedMs: 10, QueueDepth: 2, Priority: 1}
	data, _ := json.Marshal(ev)
	var m map[string]interface{}
	json.Unmarshal(data, &m)

	for _, key := range []string{"kind", "path", "priority", "waited_ms", "queue_depth"} {
		if _, ok := m[key]; !ok {
			t.Errorf("expected JSON key %q, got keys: %v", key, m)
		}
	}
}
