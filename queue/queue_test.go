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

func newTicketWithCost(priority, cost int) *Ticket {
	return &Ticket{
		Release:    make(chan struct{}, 1),
		Priority:   priority,
		EnqueuedAt: time.Now(),
		Cost:       cost,
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

// --- PopWhere tests ---

func TestFIFO_PopWhere_MatchingSubset(t *testing.T) {
	q := NewFIFO(10)
	t1 := newTicketWithCost(0, 10)
	t2 := newTicketWithCost(0, 20)
	t3 := newTicketWithCost(0, 30)
	t4 := newTicketWithCost(0, 5)
	q.Push(t1)
	q.Push(t2)
	q.Push(t3)
	q.Push(t4)

	// Pop tickets with cost <= 15.
	got := q.PopWhere(func(t *Ticket) bool { return t.Cost <= 15 })
	if len(got) != 2 {
		t.Fatalf("want 2 matched, got %d", len(got))
	}
	// FIFO order: t1 (cost=10) first, t4 (cost=5) second.
	if got[0] != t1 || got[1] != t4 {
		t.Errorf("want [t1, t4], got different tickets")
	}
	// Remaining: t2, t3 in FIFO order.
	if q.Len() != 2 {
		t.Fatalf("remaining len: got %d, want 2", q.Len())
	}
	if q.Pop() != t2 {
		t.Error("remaining: expected t2 first")
	}
	if q.Pop() != t3 {
		t.Error("remaining: expected t3 second")
	}
}

func TestFIFO_PopWhere_EmptyQueue(t *testing.T) {
	q := NewFIFO(5)
	got := q.PopWhere(func(t *Ticket) bool { return true })
	if len(got) != 0 {
		t.Errorf("empty queue: want 0 matches, got %d", len(got))
	}
}

func TestFIFO_PopWhere_NoneMatch(t *testing.T) {
	q := NewFIFO(5)
	q.Push(newTicketWithCost(0, 10))
	q.Push(newTicketWithCost(0, 20))
	got := q.PopWhere(func(t *Ticket) bool { return false })
	if len(got) != 0 {
		t.Errorf("no match: want 0, got %d", len(got))
	}
	if q.Len() != 2 {
		t.Errorf("queue should be unchanged, len: got %d, want 2", q.Len())
	}
}

func TestFIFO_PopWhere_AllMatch(t *testing.T) {
	q := NewFIFO(5)
	t1 := newTicketWithCost(0, 10)
	t2 := newTicketWithCost(0, 20)
	q.Push(t1)
	q.Push(t2)
	got := q.PopWhere(func(t *Ticket) bool { return true })
	if len(got) != 2 {
		t.Fatalf("want 2 matches, got %d", len(got))
	}
	if got[0] != t1 || got[1] != t2 {
		t.Error("all match: expected FIFO order [t1, t2]")
	}
	if q.Len() != 0 {
		t.Errorf("queue should be empty, len: %d", q.Len())
	}
}

func TestLIFO_PopWhere_MatchingSubset(t *testing.T) {
	q := NewLIFO(10)
	t1 := newTicketWithCost(0, 10)
	t2 := newTicketWithCost(0, 50)
	t3 := newTicketWithCost(0, 5)
	q.Push(t1)
	q.Push(t2)
	q.Push(t3)

	// Pop cost <= 15. LIFO scan order: t3 first, then t1. t2 stays.
	got := q.PopWhere(func(t *Ticket) bool { return t.Cost <= 15 })
	if len(got) != 2 {
		t.Fatalf("want 2 matched, got %d", len(got))
	}
	// LIFO order: t3 (newest, cost=5) first, then t1 (oldest, cost=10).
	if got[0] != t3 || got[1] != t1 {
		t.Errorf("want [t3, t1] (LIFO order), got different tickets")
	}
	// Remaining: t2 only.
	if q.Len() != 1 {
		t.Fatalf("remaining len: got %d, want 1", q.Len())
	}
	if q.Pop() != t2 {
		t.Error("remaining: expected t2")
	}
}

func TestLIFO_PopWhere_SideEffectingPredicate_FavorsNewest(t *testing.T) {
	q := NewLIFO(10)
	t1 := newTicketWithCost(0, 40) // oldest
	t2 := newTicketWithCost(0, 40)
	t3 := newTicketWithCost(0, 40) // newest
	q.Push(t1)
	q.Push(t2)
	q.Push(t3)

	// Simulate TryConsume with 60 capacity: only one 40-cost ticket fits.
	// LIFO should pick t3 (newest) since it scans back-to-front.
	remaining := 60
	got := q.PopWhere(func(t *Ticket) bool {
		if t.Cost <= remaining {
			remaining -= t.Cost
			return true
		}
		return false
	})
	if len(got) != 1 {
		t.Fatalf("want 1 matched, got %d", len(got))
	}
	if got[0] != t3 {
		t.Error("LIFO should favor newest (t3) with side-effecting predicate")
	}
}

func TestRandom_PopWhere_AllMatchedReturned(t *testing.T) {
	q := NewRandom(20)
	var small, large []*Ticket
	for i := 0; i < 10; i++ {
		s := newTicketWithCost(0, 5)
		small = append(small, s)
		q.Push(s)
	}
	for i := 0; i < 5; i++ {
		l := newTicketWithCost(0, 50)
		large = append(large, l)
		q.Push(l)
	}

	got := q.PopWhere(func(t *Ticket) bool { return t.Cost <= 10 })
	if len(got) != 10 {
		t.Fatalf("want 10 small matched, got %d", len(got))
	}
	// Verify all small tickets were returned (order doesn't matter for random).
	matched := make(map[*Ticket]bool)
	for _, m := range got {
		matched[m] = true
	}
	for _, s := range small {
		if !matched[s] {
			t.Error("missing small ticket in PopWhere result")
		}
	}
	if q.Len() != 5 {
		t.Errorf("remaining: got %d, want 5 large tickets", q.Len())
	}
}

func TestPriority_PopWhere_MatchesInPriorityOrder(t *testing.T) {
	q := NewPriority(10)
	base := time.Now()
	low := &Ticket{Release: make(chan struct{}, 1), Priority: 1, EnqueuedAt: base, Cost: 10}
	med := &Ticket{Release: make(chan struct{}, 1), Priority: 5, EnqueuedAt: base, Cost: 10}
	high := &Ticket{Release: make(chan struct{}, 1), Priority: 10, EnqueuedAt: base, Cost: 10}
	q.Push(low)
	q.Push(med)
	q.Push(high)

	// Side-effecting predicate: only 15 capacity, so only one 10-cost ticket fits.
	// Should pick highest priority (high) first.
	remaining := 15
	got := q.PopWhere(func(t *Ticket) bool {
		if t.Cost <= remaining {
			remaining -= t.Cost
			return true
		}
		return false
	})
	if len(got) != 1 {
		t.Fatalf("want 1 matched, got %d", len(got))
	}
	if got[0] != high {
		t.Errorf("expected highest priority ticket, got priority=%d", got[0].Priority)
	}
	// Remaining heap should still serve in priority order.
	if q.Len() != 2 {
		t.Fatalf("remaining: got %d, want 2", q.Len())
	}
	if q.Pop() != med {
		t.Error("remaining: expected med first")
	}
	if q.Pop() != low {
		t.Error("remaining: expected low second")
	}
}

func TestPriority_PopWhere_HeapPropertyMaintained(t *testing.T) {
	q := NewPriority(10)
	base := time.Now()
	for i := 1; i <= 5; i++ {
		q.Push(&Ticket{Release: make(chan struct{}, 1), Priority: i, EnqueuedAt: base.Add(time.Duration(i) * time.Millisecond), Cost: i * 10})
	}
	// Remove even-cost tickets (cost=20, cost=40 → priority 2 and 4).
	got := q.PopWhere(func(t *Ticket) bool { return t.Cost%20 == 0 })
	if len(got) != 2 {
		t.Fatalf("want 2 matched, got %d", len(got))
	}
	// Remaining: priority 5, 3, 1 — verify heap order.
	if q.Len() != 3 {
		t.Fatalf("remaining: got %d, want 3", q.Len())
	}
	p := q.Pop()
	if p.Priority != 5 {
		t.Errorf("first remaining: got priority %d, want 5", p.Priority)
	}
	p = q.Pop()
	if p.Priority != 3 {
		t.Errorf("second remaining: got priority %d, want 3", p.Priority)
	}
	p = q.Pop()
	if p.Priority != 1 {
		t.Errorf("third remaining: got priority %d, want 1", p.Priority)
	}
}

// --- Cost field round-trip tests ---

func TestCost_SurvivesPushPop_AllTypes(t *testing.T) {
	types := []struct {
		name string
		q    Queue
	}{
		{"fifo", NewFIFO(10)},
		{"lifo", NewLIFO(10)},
		{"random", NewRandom(10)},
		{"priority", NewPriority(10)},
	}
	for _, tt := range types {
		tk := newTicketWithCost(0, 42)
		tt.q.Push(tk)
		got := tt.q.Pop()
		if got.Cost != 42 {
			t.Errorf("%s: cost after pop: got %d, want 42", tt.name, got.Cost)
		}
	}
}
