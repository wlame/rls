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

func (q *LIFOQueue) PopWhere(fn func(t *Ticket) bool) []*Ticket {
	q.mu.Lock()
	defer q.mu.Unlock()
	// Scan back-to-front to match LIFO serve order: newest tickets are tested
	// first, so side-effecting predicates (e.g. TryConsume) favor them.
	var matched []*Ticket
	remaining := make([]*Ticket, 0, len(q.items))
	for i := len(q.items) - 1; i >= 0; i-- {
		if fn(q.items[i]) {
			matched = append(matched, q.items[i])
		} else {
			remaining = append(remaining, q.items[i])
		}
	}
	// Reverse remaining to restore original insertion order for the stack.
	for i, j := 0, len(remaining)-1; i < j; i, j = i+1, j-1 {
		remaining[i], remaining[j] = remaining[j], remaining[i]
	}
	q.items = remaining
	return matched
}

func (q *LIFOQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}
