package attach

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/wlame/rls/config"
	"github.com/wlame/rls/endpoint"
)

func testCfg() config.Config {
	return config.Config{
		Server:    config.ServerConfig{Host: "127.0.0.1", Port: 8080},
		Endpoints: []config.EndpointConfig{{Path: "/", Rate: 1}},
	}
}

func TestHub_SubscribeReceivesConfigSnapshot(t *testing.T) {
	hub := NewHub(testCfg(), nil)
	ch, unsub := hub.Subscribe()
	defer unsub()

	select {
	case msg := <-ch:
		if msg.Type != MsgConfig {
			t.Fatalf("first message type: got %q, want %q", msg.Type, MsgConfig)
		}
		var cfg config.Config
		if err := json.Unmarshal(msg.Data, &cfg); err != nil {
			t.Fatalf("unmarshal config: %v", err)
		}
		if cfg.Server.Port != 8080 {
			t.Errorf("port: got %d, want 8080", cfg.Server.Port)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for config snapshot")
	}
}

func TestHub_EventBroadcastReachesAllSubscribers(t *testing.T) {
	hub := NewHub(testCfg(), nil)
	ch1, unsub1 := hub.Subscribe()
	defer unsub1()
	ch2, unsub2 := hub.Subscribe()
	defer unsub2()

	// Drain config snapshots.
	<-ch1
	<-ch2

	events := make(chan endpoint.Event, 1)
	logs := make(chan string, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx, events, logs)

	events <- endpoint.Event{Kind: endpoint.EventServed, Path: "/", WaitedMs: 10}

	for i, ch := range []<-chan WireMsg{ch1, ch2} {
		select {
		case msg := <-ch:
			if msg.Type != MsgEvent {
				t.Errorf("sub %d: got type %q, want %q", i, msg.Type, MsgEvent)
			}
		case <-time.After(time.Second):
			t.Fatalf("sub %d: timeout", i)
		}
	}
}

func TestHub_LogBroadcastReachesAllSubscribers(t *testing.T) {
	hub := NewHub(testCfg(), nil)
	ch1, unsub1 := hub.Subscribe()
	defer unsub1()
	ch2, unsub2 := hub.Subscribe()
	defer unsub2()

	<-ch1
	<-ch2

	events := make(chan endpoint.Event, 1)
	logs := make(chan string, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx, events, logs)

	logs <- "hello log"

	for i, ch := range []<-chan WireMsg{ch1, ch2} {
		select {
		case msg := <-ch:
			if msg.Type != MsgLog {
				t.Errorf("sub %d: got type %q, want %q", i, msg.Type, MsgLog)
			}
		case <-time.After(time.Second):
			t.Fatalf("sub %d: timeout", i)
		}
	}
}

func TestHub_UnsubscribeStopsDelivery(t *testing.T) {
	hub := NewHub(testCfg(), nil)
	ch, unsub := hub.Subscribe()
	<-ch // drain config

	unsub()

	events := make(chan endpoint.Event, 1)
	logs := make(chan string, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx, events, logs)

	events <- endpoint.Event{Kind: endpoint.EventQueued, Path: "/"}

	// Give broadcast time to deliver.
	time.Sleep(50 * time.Millisecond)

	select {
	case <-ch:
		t.Error("unsubscribed channel should not receive messages")
	default:
		// expected
	}
}

func TestHub_LateSubscriberGetsConfigFirst(t *testing.T) {
	hub := NewHub(testCfg(), nil)

	events := make(chan endpoint.Event, 10)
	logs := make(chan string, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx, events, logs)

	// Send some events before subscribing.
	events <- endpoint.Event{Kind: endpoint.EventQueued, Path: "/"}

	time.Sleep(50 * time.Millisecond)

	ch, unsub := hub.Subscribe()
	defer unsub()

	msg := <-ch
	if msg.Type != MsgConfig {
		t.Errorf("late subscriber first message: got %q, want %q", msg.Type, MsgConfig)
	}
}
