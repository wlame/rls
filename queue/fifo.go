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
	q.items = q.items[1:]
	return t
}

func (q *FIFOQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}
