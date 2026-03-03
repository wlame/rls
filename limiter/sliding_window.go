package limiter

import (
	"sync"
	"time"
)

// SlidingWindowLimiter tracks timestamps in a ring buffer and fires when
// the rate within the window allows a new request.
type SlidingWindowLimiter struct {
	rps    float64
	window time.Duration
	mu     sync.Mutex
	times  []time.Time // ring buffer of recent grant times
	ch     chan time.Time
	stop   chan struct{}
}

// NewSlidingWindow creates a SlidingWindowLimiter with the given rps and window length in seconds.
func NewSlidingWindow(rps float64, windowSeconds int) *SlidingWindowLimiter {
	l := &SlidingWindowLimiter{
		rps:    rps,
		window: time.Duration(windowSeconds) * time.Second,
		ch:     make(chan time.Time, 1),
		stop:   make(chan struct{}),
	}
	go l.run()
	return l
}

func (l *SlidingWindowLimiter) run() {
	// Poll frequently; grant a tick when the window allows.
	poll := time.NewTicker(time.Millisecond)
	defer poll.Stop()
	for {
		select {
		case <-l.stop:
			return
		case now := <-poll.C:
			if l.tryGrant(now) {
				select {
				case l.ch <- now:
				case <-l.stop:
					return
				}
			}
		}
	}
}

// tryGrant removes stale entries and grants a slot if the window budget allows.
func (l *SlidingWindowLimiter) tryGrant(now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := now.Add(-l.window)
	// Remove entries older than the window.
	i := 0
	for i < len(l.times) && l.times[i].Before(cutoff) {
		i++
	}
	l.times = l.times[i:]

	// How many requests are allowed in the window?
	allowed := int(l.rps * l.window.Seconds())
	if allowed < 1 {
		allowed = 1
	}
	if len(l.times) < allowed {
		l.times = append(l.times, now)
		return true
	}
	return false
}

func (l *SlidingWindowLimiter) Tick() <-chan time.Time {
	return l.ch
}

func (l *SlidingWindowLimiter) Stop() {
	close(l.stop)
}
