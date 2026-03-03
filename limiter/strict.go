package limiter

import "time"

// StrictLimiter fires at exactly 1/rps intervals using time.Ticker.
type StrictLimiter struct {
	ticker *time.Ticker
}

// NewStrict creates a StrictLimiter that fires every 1/rps seconds.
func NewStrict(rps float64) *StrictLimiter {
	interval := time.Duration(float64(time.Second) / rps)
	return &StrictLimiter{ticker: time.NewTicker(interval)}
}

func (l *StrictLimiter) Tick() <-chan time.Time {
	return l.ticker.C
}

func (l *StrictLimiter) Stop() {
	l.ticker.Stop()
}
