package queue

import "sync"

// LIFOQueue is a bounded last-in-first-out (stack) queue.
type LIFOQueue struct {
	mu      sync.Mutex
	items   []*Ticket
	maxSize int
}

// NewLIFO creates a new LIFOQueue with the given capacity.
func NewLIFO(maxSize int) *LIFOQueue {
	return &LIFOQueue{maxSize: maxSize}
}

func (q *LIFOQueue) Push(t *Ticket) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) >= q.maxSize {
		return false
	}
	q.items = append(q.items, t)
	return true
}

func (q *LIFOQueue) Pop() *Ticket {
	q.mu.Lock()
	defer q.mu.Unlock()
	n := len(q.items)
	if n == 0 {
		return nil
	}
	t := q.items[n-1]
	q.items = q.items[:n-1]
	return t
}

func (q *LIFOQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}
