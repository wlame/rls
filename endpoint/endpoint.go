package endpoint

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/wlame/rls/config"
	"github.com/wlame/rls/limiter"
	"github.com/wlame/rls/queue"
)

// Endpoint manages a single rate-limited path.
type Endpoint struct {
	cfg     config.EndpointConfig
	queue   queue.Queue
	lim     limiter.Limiter
	ctx     context.Context
	cancel  context.CancelFunc
}

// New creates an Endpoint from its configuration, starts the dispatcher goroutine,
// and returns it ready to handle requests.
func New(cfg config.EndpointConfig) (*Endpoint, error) {
	q, err := queue.New(cfg.Scheduler, cfg.MaxQueueSize)
	if err != nil {
		return nil, err
	}

	l, err := limiter.New(cfg.Algorithm, cfg.Rate, cfg.Unit, limiter.LimiterOptions{
		BurstSize:     cfg.BurstSize,
		WindowSeconds: cfg.WindowSeconds,
	})
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	ep := &Endpoint{
		cfg:    cfg,
		queue:  q,
		lim:    l,
		ctx:    ctx,
		cancel: cancel,
	}
	go ep.dispatch()
	return ep, nil
}

// dispatch runs the rate-limited dispatch loop: on each limiter tick,
// pop the next queued ticket and signal its release channel.
func (e *Endpoint) dispatch() {
	for {
		select {
		case <-e.ctx.Done():
			return
		case <-e.lim.Tick():
			if t := e.queue.Pop(); t != nil {
				t.Release <- struct{}{}
			}
		}
	}
}

// Handle is the http.HandlerFunc for this endpoint.
func (e *Endpoint) Handle(w http.ResponseWriter, r *http.Request) {
	ticket := &queue.Ticket{
		Release:    make(chan struct{}, 1),
		Priority:   parsePriority(r),
		EnqueuedAt: time.Now(),
	}

	if e.cfg.Overflow == "block" {
		// Keep retrying until the queue accepts the ticket or the context is done.
		for {
			if e.queue.Push(ticket) {
				break
			}
			select {
			case <-e.ctx.Done():
				http.Error(w, "service shutting down", http.StatusServiceUnavailable)
				return
			case <-time.After(time.Millisecond):
				// retry
			}
		}
	} else {
		// Default: reject when full.
		if !e.queue.Push(ticket) {
			writeError(w, http.StatusTooManyRequests, "queue full")
			return
		}
	}

	// Block until the dispatcher releases this ticket.
	select {
	case <-ticket.Release:
	case <-e.ctx.Done():
		http.Error(w, "service shutting down", http.StatusServiceUnavailable)
		return
	}

	resp := buildResponse(e.cfg, e.queue.Len(), ticket.EnqueuedAt)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}

// Stop shuts down the dispatcher and limiter.
func (e *Endpoint) Stop() {
	e.cancel()
	e.lim.Stop()
}

// parsePriority reads the X-Priority request header. Defaults to 0.
func parsePriority(r *http.Request) int {
	v := r.Header.Get("X-Priority")
	if v == "" {
		return 0
	}
	p, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return p
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": msg}) //nolint:errcheck
}
