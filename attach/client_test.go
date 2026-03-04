package attach

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/wlame/rls/endpoint"
)

func TestConnectAddr_Integration(t *testing.T) {
	cfg := testCfg()
	cfg.Server.Host = "0.0.0.0" // should be patched to 127.0.0.1
	hub := NewHub(cfg)

	events := make(chan endpoint.Event, 10)
	logs := make(chan string, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx, events, logs)

	sockPath := filepath.Join(t.TempDir(), "test.sock")
	go Serve(ctx, hub, sockPath)
	time.Sleep(50 * time.Millisecond)

	gotCfg, evCh, logCh, err := ConnectAddr(sockPath)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	// Verify 0.0.0.0 → 127.0.0.1 patch.
	if gotCfg.Server.Host != "127.0.0.1" {
		t.Errorf("host patch: got %q, want 127.0.0.1", gotCfg.Server.Host)
	}

	// Send event, verify arrival.
	events <- endpoint.Event{Kind: endpoint.EventServed, Path: "/", WaitedMs: 7}

	select {
	case ev := <-evCh:
		if ev.WaitedMs != 7 {
			t.Errorf("waited_ms: got %d, want 7", ev.WaitedMs)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}

	// Send log, verify arrival.
	logs <- "test log line"

	select {
	case line := <-logCh:
		if line != "test log line" {
			t.Errorf("log: got %q, want %q", line, "test log line")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for log")
	}
}

func TestConnect_NonExistentPID(t *testing.T) {
	_, _, _, err := Connect(99999)
	if err == nil {
		t.Fatal("expected error for non-existent PID")
	}
}
