package limiter

import (
	"fmt"
	"time"
)

// Limiter controls when rate-limited slots are made available.
type Limiter interface {
	// Tick returns a channel that fires each time a request slot is available.
	// One receive = one request may proceed.
	Tick() <-chan time.Time
	// Stop releases resources (stops ticker goroutines, etc.)
	Stop()
}

// LimiterOptions carries algorithm-specific configuration.
type LimiterOptions struct {
	BurstSize     int // token_bucket: max accumulated tokens
	WindowSeconds int // sliding_window: observation window length
}

// New creates a Limiter for the given algorithm, rate, and unit.
// Valid algorithms: "strict", "token_bucket", "sliding_window".
// Valid units: "rps", "rpm".
func New(algorithm string, rate float64, unit string, opts LimiterOptions) (Limiter, error) {
	rps := toRPS(rate, unit)
	if rps <= 0 {
		return nil, fmt.Errorf("rate must be > 0, got %f %s", rate, unit)
	}

	switch algorithm {
	case "strict":
		return NewStrict(rps), nil
	case "token_bucket":
		burst := opts.BurstSize
		if burst <= 0 {
			burst = 1
		}
		return NewTokenBucket(rps, burst), nil
	case "sliding_window":
		win := opts.WindowSeconds
		if win <= 0 {
			win = 1
		}
		return NewSlidingWindow(rps, win), nil
	default:
		return nil, fmt.Errorf("unknown algorithm %q; valid: strict, token_bucket, sliding_window", algorithm)
	}
}

// toRPS converts a rate in the given unit to requests per second.
func toRPS(rate float64, unit string) float64 {
	if unit == "rpm" {
		return rate / 60.0
	}
	return rate
}
