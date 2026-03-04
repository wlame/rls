package attach

import (
	"encoding/json"
	"testing"

	"github.com/wlame/rls/config"
	"github.com/wlame/rls/endpoint"
)

func TestWireMsg_ConfigRoundTrip(t *testing.T) {
	cfg := config.Config{
		Server:    config.ServerConfig{Host: "127.0.0.1", Port: 8080},
		Endpoints: []config.EndpointConfig{{Path: "/", Rate: 1}},
	}
	data, _ := json.Marshal(cfg)
	msg := WireMsg{Type: MsgConfig, Data: data}

	encoded, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got WireMsg
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Type != MsgConfig {
		t.Errorf("type: got %q, want %q", got.Type, MsgConfig)
	}
	var gotCfg config.Config
	if err := json.Unmarshal(got.Data, &gotCfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if gotCfg.Server.Port != 8080 {
		t.Errorf("port: got %d, want 8080", gotCfg.Server.Port)
	}
}

func TestWireMsg_EventRoundTrip(t *testing.T) {
	ev := endpoint.Event{Kind: endpoint.EventServed, Path: "/fast", WaitedMs: 42, QueueDepth: 3}
	data, _ := json.Marshal(ev)
	msg := WireMsg{Type: MsgEvent, Data: data}

	encoded, _ := json.Marshal(msg)
	var got WireMsg
	json.Unmarshal(encoded, &got)

	if got.Type != MsgEvent {
		t.Errorf("type: got %q, want %q", got.Type, MsgEvent)
	}
	var gotEv endpoint.Event
	json.Unmarshal(got.Data, &gotEv)
	if gotEv != ev {
		t.Errorf("event mismatch: got %+v, want %+v", gotEv, ev)
	}
}

func TestWireMsg_LogRoundTrip(t *testing.T) {
	logLine := "2026-03-04 12:00:00.000  serve /fast"
	data, _ := json.Marshal(logLine)
	msg := WireMsg{Type: MsgLog, Data: data}

	encoded, _ := json.Marshal(msg)
	var got WireMsg
	json.Unmarshal(encoded, &got)

	if got.Type != MsgLog {
		t.Errorf("type: got %q, want %q", got.Type, MsgLog)
	}
	var gotLine string
	json.Unmarshal(got.Data, &gotLine)
	if gotLine != logLine {
		t.Errorf("log: got %q, want %q", gotLine, logLine)
	}
}
