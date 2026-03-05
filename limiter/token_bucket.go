package limiter

import (
	"context"

	"golang.org/x/time/rate"
)

// TokenBucketLimiter wraps golang.org/x/time/rate and allows controlled bursting.
type TokenBucketLimiter struct {
	rl *rate.Limiter
}

// NewTokenBucket creates a TokenBucketLimiter with the given rps and burst size.
func NewTokenBucket(rps float64, burst int) *TokenBucketLimiter {
	return &TokenBucketLimiter{
		rl: rate.NewLimiter(rate.Limit(rps), burst),
	}
}

func (l *TokenBucketLimiter) Wait(ctx context.Context) error {
	return l.rl.Wait(ctx)
}

func (l *TokenBucketLimiter) TokensAvailable() int {
	return int(l.rl.Tokens())
}

func (l *TokenBucketLimiter) Stop() {}
