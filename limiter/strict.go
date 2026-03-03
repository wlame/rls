package limiter

import (
	"context"
	"time"
)

// StrictLimiter fires at exactly 1/rps intervals using time.Ticker.
type StrictLimiter struct {
	ticker *time.Ticker
}

// NewStrict creates a StrictLimiter that fires every 1/rps seconds.
func NewStrict(rps float64) *StrictLimiter {
	interval := time.Duration(float64(time.Second) / rps)
	return &StrictLimiter{ticker: time.NewTicker(interval)}
}

func (l *StrictLimiter) Wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-l.ticker.C:
		return nil
	}
}

func (l *StrictLimiter) Stop() {
	l.ticker.Stop()
}
