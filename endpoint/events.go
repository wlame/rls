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
	Kind       EventKind
	Path       string
	Priority   int   // populated for EventQueued
	WaitedMs   int64 // populated for EventServed
	QueueDepth int   // populated for EventServed
}

// Option is a functional option for configuring an Endpoint.
type Option func(*Endpoint)

// WithEventSink configures the endpoint to emit Events to ch.
// ch must be buffered; events are dropped (never block) if ch is full.
func WithEventSink(ch chan<- Event) Option {
	return func(e *Endpoint) { e.events = ch }
}
