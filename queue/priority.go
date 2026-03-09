package queue

import (
	"container/heap"
	"sync"
)

// priorityHeap implements heap.Interface for Tickets.
// Higher Priority value = popped first; ties broken by EnqueuedAt (earlier = first).
type priorityHeap []*Ticket

func (h priorityHeap) Len() int { return len(h) }

func (h priorityHeap) Less(i, j int) bool {
	if h[i].Priority != h[j].Priority {
		return h[i].Priority > h[j].Priority // higher priority first
	}
	return h[i].EnqueuedAt.Before(h[j].EnqueuedAt) // earlier enqueue time first
}

func (h priorityHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *priorityHeap) Push(x any) {
	*h = append(*h, x.(*Ticket))
}

func (h *priorityHeap) Pop() any {
	old := *h
	n := len(old)
	t := old[n-1]
	old[n-1] = nil
	*h = old[:n-1]
	return t
}

// PriorityQueue serves highest-priority tickets first; ties broken by enqueue order.
type PriorityQueue struct {
	mu      sync.Mutex
	h       priorityHeap
	maxSize int
}

// NewPriority creates a new PriorityQueue with the given capacity.
func NewPriority(maxSize int) *PriorityQueue {
	h := make(priorityHeap, 0, maxSize)
	heap.Init(&h)
	return &PriorityQueue{h: h, maxSize: maxSize}
}

func (q *PriorityQueue) Push(t *Ticket) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.h.Len() >= q.maxSize {
		return false
	}
	heap.Push(&q.h, t)
	return true
}

func (q *PriorityQueue) Pop() *Ticket {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.h.Len() == 0 {
		return nil
	}
	return heap.Pop(&q.h).(*Ticket)
}

func (q *PriorityQueue) PopWhere(fn func(t *Ticket) bool) []*Ticket {
	q.mu.Lock()
	defer q.mu.Unlock()
	// Pop all items in priority order, test predicate, collect matches,
	// re-push non-matches. This ensures side-effecting predicates
	// (e.g. TryConsume) favor highest-priority tickets first.
	var matched, keep []*Ticket
	for q.h.Len() > 0 {
		t := heap.Pop(&q.h).(*Ticket)
		if fn(t) {
			matched = append(matched, t)
		} else {
			keep = append(keep, t)
		}
	}
	for _, t := range keep {
		heap.Push(&q.h, t)
	}
	return matched
}

func (q *PriorityQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.h.Len()
}
