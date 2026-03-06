package limiter

import (
	"context"
	"fmt"
	"math"
)

// Limiter controls when rate-limited slots are made available.
type Limiter interface {
	// Wait blocks until a request slot is available or ctx is cancelled.
	// Returns ctx.Err() if the context is cancelled, nil otherwise.
	Wait(ctx context.Context) error
	// Stop releases resources (stops ticker goroutines, etc.)
	Stop()
}

// BurstQuerier is implemented by limiters that support burst tokens.
type BurstQuerier interface {
	TokensAvailable() int
}

// LimiterOptions carries algorithm-specific configuration.
type LimiterOptions struct {
	BurstSize      int     // token_bucket: max accumulated tokens
	WindowSeconds  int     // sliding_window: observation window length
	CompensationMs float64 // latency compensation: release tickets early by this many ms
}

// New creates a Limiter for the given algorithm, rate, and unit.
// Valid algorithms: "strict", "token_bucket", "sliding_window".
// Valid units: "rps", "rpm".
func New(algorithm string, rate float64, unit string, opts LimiterOptions) (Limiter, error) {
	rps := toRPS(rate, unit)
	if rps <= 0 {
		return nil, fmt.Errorf("rate must be > 0, got %f %s", rate, unit)
	}

	if opts.CompensationMs > 0 {
		interval := 1.0/rps - opts.CompensationMs/1000.0
		rps = 1.0 / math.Max(0.001, interval)
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
