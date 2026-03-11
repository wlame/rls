package queue

import "sync"

// FIFOQueue is a bounded first-in-first-out queue.
type FIFOQueue struct {
	mu      sync.Mutex
	items   []*Ticket
	maxSize int
}

// NewFIFO creates a new FIFOQueue with the given capacity.
func NewFIFO(maxSize int) *FIFOQueue {
	return &FIFOQueue{maxSize: maxSize}
}

func (q *FIFOQueue) Push(t *Ticket) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) >= q.maxSize {
		return false
	}
	q.items = append(q.items, t)
	return true
}

func (q *FIFOQueue) Pop() *Ticket {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return nil
	}
	t := q.items[0]
	q.items[0] = nil // GC hint
	q.items = q.items[1:]
	// Compact when usage drops below 1/4 of backing array capacity
	// and the array has grown beyond maxSize.
	if cap(q.items) > q.maxSize && len(q.items) < cap(q.items)/4 {
		compact := make([]*Ticket, len(q.items))
		copy(compact, q.items)
		q.items = compact
	}
	return t
}

func (q *FIFOQueue) PopWhere(fn func(t *Ticket) bool) []*Ticket {
	q.mu.Lock()
	defer q.mu.Unlock()
	var matched []*Ticket
	remaining := q.items[:0]
	for _, t := range q.items {
		if fn(t) {
			matched = append(matched, t)
		} else {
			remaining = append(remaining, t)
		}
	}
	q.items = remaining
	return matched
}

func (q *FIFOQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}
