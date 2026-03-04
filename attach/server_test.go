package attach

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/wlame/rls/endpoint"
)

func TestServe_Integration(t *testing.T) {
	hub := NewHub(testCfg())

	events := make(chan endpoint.Event, 10)
	logs := make(chan string, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx, events, logs)

	sockPath := filepath.Join(t.TempDir(), "test.sock")

	// Start server in background.
	serveErr := make(chan error, 1)
	go func() { serveErr <- Serve(ctx, hub, sockPath) }()

	// Wait for socket to appear.
	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	scanner := bufio.NewScanner(conn)

	// First message should be config.
	if !scanner.Scan() {
		t.Fatal("expected config message")
	}
	var msg WireMsg
	if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if msg.Type != MsgConfig {
		t.Errorf("first message type: got %q, want %q", msg.Type, MsgConfig)
	}

	// Send an event through hub, verify it arrives.
	events <- endpoint.Event{Kind: endpoint.EventServed, Path: "/", WaitedMs: 99}

	if !scanner.Scan() {
		t.Fatal("expected event message")
	}
	json.Unmarshal(scanner.Bytes(), &msg)
	if msg.Type != MsgEvent {
		t.Errorf("second message type: got %q, want %q", msg.Type, MsgEvent)
	}

	cancel()
}

func TestSocketPath(t *testing.T) {
	got := SocketPath(12345)
	want := "/tmp/rls-12345.sock"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
