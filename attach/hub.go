package attach

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/wlame/rls/config"
	"github.com/wlame/rls/endpoint"
)

// Hub broadcasts events and logs to N subscribers.
// Each new subscriber receives a config snapshot as its first message.
type Hub struct {
	cfg  config.Config
	mu   sync.RWMutex
	subs map[chan WireMsg]struct{}
}

// NewHub creates a Hub that sends cfg as the initial snapshot to each subscriber.
func NewHub(cfg config.Config) *Hub {
	return &Hub{
		cfg:  cfg,
		subs: make(map[chan WireMsg]struct{}),
	}
}

// Subscribe returns a channel that receives all broadcast messages (config snapshot first)
// and an unsubscribe function. The returned channel is buffered (256).
func (h *Hub) Subscribe() (<-chan WireMsg, func()) {
	ch := make(chan WireMsg, 256)

	h.mu.Lock()
	h.subs[ch] = struct{}{}

	// Send config snapshot while holding the lock — guarantees no broadcast
	// events can arrive before the config snapshot.
	data, _ := json.Marshal(h.cfg)
	select {
	case ch <- WireMsg{Type: MsgConfig, Data: data}:
	default:
	}
	h.mu.Unlock()

	unsub := func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
	}
	return ch, unsub
}

// Run drains events and logs channels, broadcasting each as a WireMsg.
// It returns when ctx is cancelled.
func (h *Hub) Run(ctx context.Context, events <-chan endpoint.Event, logs <-chan string) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			data, _ := json.Marshal(ev)
			h.broadcast(WireMsg{Type: MsgEvent, Data: data})
		case line, ok := <-logs:
			if !ok {
				return
			}
			data, _ := json.Marshal(line)
			h.broadcast(WireMsg{Type: MsgLog, Data: data})
		}
	}
}

// broadcast sends msg to all subscribers (non-blocking; drops on full).
func (h *Hub) broadcast(msg WireMsg) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs {
		select {
		case ch <- msg:
		default:
		}
	}
}
