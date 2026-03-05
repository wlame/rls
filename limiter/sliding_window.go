package limiter

import (
	"context"
	"sync"
	"time"
)

// SlidingWindowLimiter tracks timestamps in a ring buffer and grants a slot when
// the rate within the window allows a new request.
type SlidingWindowLimiter struct {
	rps    float64
	window time.Duration
	mu     sync.Mutex
	times  []time.Time // ring buffer of recent grant times
}

// NewSlidingWindow creates a SlidingWindowLimiter with the given rps and window length in seconds.
func NewSlidingWindow(rps float64, windowSeconds int) *SlidingWindowLimiter {
	return &SlidingWindowLimiter{
		rps:    rps,
		window: time.Duration(windowSeconds) * time.Second,
	}
}

// Wait blocks until a slot is available within the sliding window or ctx is cancelled.
func (l *SlidingWindowLimiter) Wait(ctx context.Context) error {
	timer := time.NewTimer(time.Millisecond)
	defer timer.Stop()
	for {
		if l.tryGrant(time.Now()) {
			return nil
		}
		timer.Reset(time.Millisecond)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
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

func (l *SlidingWindowLimiter) Stop() {}
