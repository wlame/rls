package endpoint

// EventKind identifies the type of rate-limiting event.
type EventKind uint8

const (
	EventQueued   EventKind = iota // request entered the queue
	EventServed                    // request was released and served
	EventRejected                  // request was rejected (queue full → 429)
)

// Event carries rate-limiting telemetry emitted by an endpoint handler.
// Events are sent on a best-effort basis: if the sink channel is full, the event is dropped.
type Event struct {
	Kind       EventKind `json:"kind"`
	Path       string    `json:"path"`
	Priority   int       `json:"priority"`    // populated for EventQueued
	WaitedMs   int64     `json:"waited_ms"`   // populated for EventServed
	QueueDepth int       `json:"queue_depth"` // populated for EventServed
}

// Option is a functional option for configuring an Endpoint.
type Option func(*Endpoint)

// WithEventSink configures the endpoint to emit Events to ch.
// ch must be buffered; events are dropped (never block) if ch is full.
func WithEventSink(ch chan<- Event) Option {
	return func(e *Endpoint) { e.events = ch }
}
