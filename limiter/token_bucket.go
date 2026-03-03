package limiter

import (
	"context"
	"time"

	"golang.org/x/time/rate"
)

// TokenBucketLimiter wraps golang.org/x/time/rate and allows controlled bursting.
type TokenBucketLimiter struct {
	rl     *rate.Limiter
	ch     chan time.Time
	ctx    context.Context
	cancel context.CancelFunc
}

// NewTokenBucket creates a TokenBucketLimiter with the given rps and burst size.
func NewTokenBucket(rps float64, burst int) *TokenBucketLimiter {
	ctx, cancel := context.WithCancel(context.Background())
	l := &TokenBucketLimiter{
		rl:     rate.NewLimiter(rate.Limit(rps), burst),
		ch:     make(chan time.Time, burst),
		ctx:    ctx,
		cancel: cancel,
	}
	go l.run()
	return l
}

func (l *TokenBucketLimiter) run() {
	for {
		if err := l.rl.Wait(l.ctx); err != nil {
			return // context cancelled
		}
		select {
		case l.ch <- time.Now():
		case <-l.ctx.Done():
			return
		}
	}
}

func (l *TokenBucketLimiter) Tick() <-chan time.Time {
	return l.ch
}

func (l *TokenBucketLimiter) Stop() {
	l.cancel()
}
