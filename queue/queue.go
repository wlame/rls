package queue

import (
	"fmt"
	"time"
)

// New creates a Queue of the given scheduler type with the given max capacity.
// Valid schedulers: "fifo", "lifo", "priority", "random".
func New(scheduler string, maxSize int) (Queue, error) {
	switch scheduler {
	case "fifo":
		return NewFIFO(maxSize), nil
	case "lifo":
		return NewLIFO(maxSize), nil
	case "priority":
		return NewPriority(maxSize), nil
	case "random":
		return NewRandom(maxSize), nil
	default:
		return nil, fmt.Errorf("unknown scheduler %q; valid: fifo, lifo, priority, random", scheduler)
	}
}

// Ticket represents a queued request waiting for a rate-limit slot.
type Ticket struct {
	Release    chan struct{} // dispatcher closes/sends to release the waiting handler
	Priority   int          // higher = served sooner; used by PriorityQueue only
	EnqueuedAt time.Time
}

// Queue is the interface implemented by all scheduling strategies.
type Queue interface {
	// Push enqueues t. Returns false if the queue is full (caller should 429).
	Push(t *Ticket) bool
	// Pop removes and returns the next ticket, or nil if the queue is empty.
	Pop() *Ticket
	// Len returns the current number of queued tickets.
	Len() int
}
