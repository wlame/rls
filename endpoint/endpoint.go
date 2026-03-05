package endpoint

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/wlame/rls/config"
	"github.com/wlame/rls/limiter"
	"github.com/wlame/rls/queue"
)

// Endpoint manages a single rate-limited path.
type Endpoint struct {
	cfg    config.EndpointConfig
	queue  queue.Queue
	lim    limiter.Limiter
	work   chan struct{} // signals dispatch that a ticket was just pushed
	ctx    context.Context
	cancel context.CancelFunc
	events chan<- Event // nil when no sink configured
}

// New creates an Endpoint from its configuration, starts the dispatcher goroutine,
// and returns it ready to handle requests.
func New(cfg config.EndpointConfig, opts ...Option) (*Endpoint, error) {
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
		work:   make(chan struct{}, cfg.MaxQueueSize),
		ctx:    ctx,
		cancel: cancel,
	}
	for _, opt := range opts {
		opt(ep)
	}
	go ep.dispatch()
	return ep, nil
}

// emit sends an event to the configured sink. It never blocks: events are dropped if the
// channel is full. Safe to call when events is nil (no-op).
func (e *Endpoint) emit(ev Event) {
	if e.events == nil {
		return
	}
	select {
	case e.events <- ev:
	default:
	}
}

// dispatch runs the rate-limited dispatch loop: wait for a queued ticket,
// then call Wait() on the limiter to consume one slot before releasing it.
// Waiting for work before calling Wait() is critical for burst-capable limiters
// (token bucket): Wait() only consumes a token when a request is actually ready.
func (e *Endpoint) dispatch() {
	for {
		// Block until a ticket has been pushed into the queue.
		select {
		case <-e.ctx.Done():
			return
		case <-e.work:
		}
		// Consume one rate-limiter slot, then release the ticket.
		if err := e.lim.Wait(e.ctx); err != nil {
			return
		}
		if t := e.queue.Pop(); t != nil {
			t.Release <- struct{}{}
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

	// Admission timeout: predict wait and reject early if it exceeds the threshold.
	timeout := e.cfg.QueueTimeout
	if v := r.URL.Query().Get("timeout"); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil && parsed >= 0 {
			timeout = parsed
		}
	}
	if timeout > 0 {
		if est := e.estimateWait(); est > time.Duration(timeout*float64(time.Second)) {
			e.emit(Event{Kind: EventRejected, Path: e.cfg.Path})
			writeError(w, http.StatusTooManyRequests, "estimated wait exceeds timeout")
			return
		}
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
			e.emit(Event{Kind: EventRejected, Path: e.cfg.Path})
			writeError(w, http.StatusTooManyRequests, "queue full")
			return
		}
	}
	e.emit(Event{Kind: EventQueued, Path: e.cfg.Path, Priority: ticket.Priority})
	// Notify the dispatcher that a ticket is ready.
	// Non-blocking: work is sized to MaxQueueSize so it can never be full
	// when a push just succeeded.
	e.work <- struct{}{}

	// Block until the dispatcher releases this ticket, the server shuts down,
	// or the client disconnects.
	select {
	case <-ticket.Release:
	case <-e.ctx.Done():
		http.Error(w, "service shutting down", http.StatusServiceUnavailable)
		return
	case <-r.Context().Done():
		// Client disconnected. The ticket remains in the queue and the dispatcher
		// will eventually pop and release it (harmless: Release is buffered).
		return
	}

	resp := buildResponse(e.cfg, e.queue.Len(), ticket.EnqueuedAt)
	e.emit(Event{Kind: EventServed, Path: e.cfg.Path, WaitedMs: resp.QueuedForMs, QueueDepth: resp.QueueDepth})
	req := r.URL.RawQuery
	if req == "" {
		req = "-"
	}
	log.Printf("%s  serve  %-12s  waited=%6dms  queue=%2d  rate=%.0f%s  [%s/%s]  %s",
		time.Now().Format("2006-01-02 15:04:05.000"),
		resp.Endpoint, resp.QueuedForMs, resp.QueueDepth,
		resp.Rate, resp.Unit,
		resp.Scheduler, resp.Algorithm,
		req)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}

// QueueLen returns the current number of tickets in the queue.
func (e *Endpoint) QueueLen() int {
	return e.queue.Len()
}

// Path returns the configured path for this endpoint.
func (e *Endpoint) Path() string {
	return e.cfg.Path
}

// Stop shuts down the dispatcher and limiter.
func (e *Endpoint) Stop() {
	e.cancel()
	e.lim.Stop()
}

// estimateWait predicts how long a new request would wait in the queue.
// For LIFO and random schedulers, prediction is not meaningful so it returns 0.
func (e *Endpoint) estimateWait() time.Duration {
	sched := e.cfg.Scheduler
	if sched == "lifo" || sched == "random" {
		return 0
	}

	rps := e.cfg.Rate
	if e.cfg.Unit == "rpm" {
		rps = e.cfg.Rate / 60.0
	}
	if rps <= 0 {
		return 0
	}

	ahead := e.queue.Len()

	if bq, ok := e.lim.(limiter.BurstQuerier); ok {
		available := bq.TokensAvailable()
		ahead = int(math.Max(0, float64(ahead-available)))
	}

	return time.Duration(float64(ahead) / rps * float64(time.Second))
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
