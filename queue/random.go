package queue

import (
	"math/rand"
	"sync"
)

// RandomQueue serves tickets in random order.
type RandomQueue struct {
	mu      sync.Mutex
	items   []*Ticket
	maxSize int
}

// NewRandom creates a new RandomQueue with the given capacity.
func NewRandom(maxSize int) *RandomQueue {
	return &RandomQueue{maxSize: maxSize}
}

func (q *RandomQueue) Push(t *Ticket) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) >= q.maxSize {
		return false
	}
	q.items = append(q.items, t)
	return true
}

func (q *RandomQueue) Pop() *Ticket {
	q.mu.Lock()
	defer q.mu.Unlock()
	n := len(q.items)
	if n == 0 {
		return nil
	}
	i := rand.Intn(n)
	t := q.items[i]
	// swap with last and shrink
	q.items[i] = q.items[n-1]
	q.items[n-1] = nil
	q.items = q.items[:n-1]
	return t
}

func (q *RandomQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}
