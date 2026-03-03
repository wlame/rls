package queue

import (
	"testing"
	"time"
)

func newTicket(priority int) *Ticket {
	return &Ticket{
		Release:    make(chan struct{}, 1),
		Priority:   priority,
		EnqueuedAt: time.Now(),
	}
}

// --- FIFO tests ---

func TestFIFO_PushPop_Order(t *testing.T) {
	q := NewFIFO(10)
	t1, t2, t3 := newTicket(0), newTicket(0), newTicket(0)
	q.Push(t1)
	q.Push(t2)
	q.Push(t3)

	if got := q.Pop(); got != t1 {
		t.Error("FIFO: expected t1 first")
	}
	if got := q.Pop(); got != t2 {
		t.Error("FIFO: expected t2 second")
	}
	if got := q.Pop(); got != t3 {
		t.Error("FIFO: expected t3 third")
	}
	if got := q.Pop(); got != nil {
		t.Error("FIFO: expected nil on empty")
	}
}

func TestFIFO_Overflow(t *testing.T) {
	q := NewFIFO(2)
	if !q.Push(newTicket(0)) {
		t.Fatal("first push should succeed")
	}
	if !q.Push(newTicket(0)) {
		t.Fatal("second push should succeed")
	}
	if q.Push(newTicket(0)) {
		t.Fatal("third push should fail (overflow)")
	}
}

func TestFIFO_Len(t *testing.T) {
	q := NewFIFO(10)
	if q.Len() != 0 {
		t.Errorf("empty len: got %d, want 0", q.Len())
	}
	q.Push(newTicket(0))
	q.Push(newTicket(0))
	if q.Len() != 2 {
		t.Errorf("len after 2 pushes: got %d, want 2", q.Len())
	}
	q.Pop()
	if q.Len() != 1 {
		t.Errorf("len after pop: got %d, want 1", q.Len())
	}
}

func TestFIFO_PopEmpty(t *testing.T) {
	q := NewFIFO(5)
	if got := q.Pop(); got != nil {
		t.Error("pop on empty FIFO should return nil")
	}
}

// --- LIFO tests ---

func TestLIFO_PushPop_Order(t *testing.T) {
	q := NewLIFO(10)
	t1, t2, t3 := newTicket(0), newTicket(0), newTicket(0)
	q.Push(t1)
	q.Push(t2)
	q.Push(t3)

	if got := q.Pop(); got != t3 {
		t.Error("LIFO: expected t3 first (last in)")
	}
	if got := q.Pop(); got != t2 {
		t.Error("LIFO: expected t2 second")
	}
	if got := q.Pop(); got != t1 {
		t.Error("LIFO: expected t1 third")
	}
	if got := q.Pop(); got != nil {
		t.Error("LIFO: expected nil on empty")
	}
}

func TestLIFO_Overflow(t *testing.T) {
	q := NewLIFO(2)
	q.Push(newTicket(0))
	q.Push(newTicket(0))
	if q.Push(newTicket(0)) {
		t.Fatal("push on full LIFO should fail")
	}
}

func TestLIFO_Len(t *testing.T) {
	q := NewLIFO(10)
	if q.Len() != 0 {
		t.Errorf("empty len: got %d, want 0", q.Len())
	}
	q.Push(newTicket(0))
	q.Push(newTicket(0))
	if q.Len() != 2 {
		t.Errorf("len after 2 pushes: got %d, want 2", q.Len())
	}
	q.Pop()
	if q.Len() != 1 {
		t.Errorf("len after pop: got %d, want 1", q.Len())
	}
}

func TestLIFO_PopEmpty(t *testing.T) {
	q := NewLIFO(5)
	if got := q.Pop(); got != nil {
		t.Error("pop on empty LIFO should return nil")
	}
}

// --- Priority queue tests ---

func TestPriority_HigherPriorityFirst(t *testing.T) {
	q := NewPriority(10)
	low := newTicket(1)
	high := newTicket(10)
	medium := newTicket(5)

	q.Push(low)
	q.Push(high)
	q.Push(medium)

	if got := q.Pop(); got != high {
		t.Error("priority: expected high first")
	}
	if got := q.Pop(); got != medium {
		t.Error("priority: expected medium second")
	}
	if got := q.Pop(); got != low {
		t.Error("priority: expected low third")
	}
}

func TestPriority_EqualPriorityFIFO(t *testing.T) {
	q := NewPriority(10)
	// Use different enqueue times by sleeping briefly isn't reliable in tests;
	// instead manually set EnqueuedAt.
	base := time.Now()
	t1 := &Ticket{Release: make(chan struct{}, 1), Priority: 5, EnqueuedAt: base}
	t2 := &Ticket{Release: make(chan struct{}, 1), Priority: 5, EnqueuedAt: base.Add(time.Millisecond)}
	t3 := &Ticket{Release: make(chan struct{}, 1), Priority: 5, EnqueuedAt: base.Add(2 * time.Millisecond)}

	q.Push(t1)
	q.Push(t2)
	q.Push(t3)

	if got := q.Pop(); got != t1 {
		t.Error("equal priority: expected t1 (earliest enqueue) first")
	}
	if got := q.Pop(); got != t2 {
		t.Error("equal priority: expected t2 second")
	}
}

func TestPriority_Overflow(t *testing.T) {
	q := NewPriority(2)
	q.Push(newTicket(0))
	q.Push(newTicket(0))
	if q.Push(newTicket(0)) {
		t.Fatal("push on full priority queue should fail")
	}
}

func TestPriority_Len(t *testing.T) {
	q := NewPriority(10)
	if q.Len() != 0 {
		t.Errorf("empty len: got %d", q.Len())
	}
	q.Push(newTicket(0))
	q.Push(newTicket(0))
	if q.Len() != 2 {
		t.Errorf("len after 2 pushes: got %d, want 2", q.Len())
	}
}

// --- Random queue tests ---

func TestRandom_AllItemsPopped(t *testing.T) {
	const n = 20
	q := NewRandom(n)
	tickets := make([]*Ticket, n)
	for i := range tickets {
		tickets[i] = newTicket(0)
		q.Push(tickets[i])
	}

	seen := make(map[*Ticket]bool)
	for q.Len() > 0 {
		got := q.Pop()
		if got == nil {
			t.Fatal("pop returned nil before queue empty")
		}
		if seen[got] {
			t.Fatal("same ticket returned twice")
		}
		seen[got] = true
	}
	if len(seen) != n {
		t.Errorf("popped %d items, want %d", len(seen), n)
	}
	if q.Pop() != nil {
		t.Error("pop on empty random queue should return nil")
	}
}

func TestRandom_Overflow(t *testing.T) {
	q := NewRandom(2)
	q.Push(newTicket(0))
	q.Push(newTicket(0))
	if q.Push(newTicket(0)) {
		t.Fatal("push on full random queue should fail")
	}
}

func TestRandom_Len(t *testing.T) {
	q := NewRandom(10)
	if q.Len() != 0 {
		t.Errorf("empty len: got %d", q.Len())
	}
	q.Push(newTicket(0))
	if q.Len() != 1 {
		t.Errorf("len after push: got %d, want 1", q.Len())
	}
	q.Pop()
	if q.Len() != 0 {
		t.Errorf("len after pop: got %d, want 0", q.Len())
	}
}

// --- Factory tests ---

func TestNew_ReturnsCorrectTypes(t *testing.T) {
	cases := []struct {
		scheduler string
	}{
		{"fifo"},
		{"lifo"},
		{"priority"},
		{"random"},
	}
	for _, tc := range cases {
		q, err := New(tc.scheduler, 10)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", tc.scheduler, err)
		}
		if q == nil {
			t.Errorf("%s: expected non-nil queue", tc.scheduler)
		}
	}
}

func TestNew_UnknownScheduler(t *testing.T) {
	_, err := New("unknown", 10)
	if err == nil {
		t.Fatal("expected error for unknown scheduler, got nil")
	}
}
