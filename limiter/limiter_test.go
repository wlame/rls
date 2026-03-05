package limiter

import (
	"context"
	"testing"
	"time"
)

// --- StrictLimiter tests ---

func TestStrict_FiresAtApproximateRate(t *testing.T) {
	// 10 RPS → expect ~5 waits in 500ms (±2 tolerance)
	l := NewStrict(10)
	defer l.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	count := 0
	for {
		if err := l.Wait(ctx); err != nil {
			break
		}
		count++
	}

	// Expect 4–6 (10 RPS × 0.5s = 5, ±20%)
	if count < 3 || count > 7 {
		t.Errorf("strict 10 RPS: got %d in 500ms, want ~5", count)
	}
}

func TestStrict_Stop_DoesNotPanic(t *testing.T) {
	l := NewStrict(100)
	l.Stop()
	l.Stop() // double-stop should not panic
}

// --- TokenBucketLimiter tests ---

func TestTokenBucket_BurstReleasesUpToN(t *testing.T) {
	// burst=5 means up to 5 slots available immediately.
	l := NewTokenBucket(100, 5)
	defer l.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	count := 0
	for {
		if err := l.Wait(ctx); err != nil {
			break
		}
		count++
	}
	// With burst=5 and 100 RPS, 100ms window: expect 5+ grants.
	if count < 5 {
		t.Errorf("token bucket burst=5: got %d in 100ms, want ≥5", count)
	}
}

func TestTokenBucket_ThrottlesAfterBurst(t *testing.T) {
	// 2 RPS, burst=2 → first 2 immediate, then ~1 per 500ms
	l := NewTokenBucket(2, 2)
	defer l.Stop()

	ctx := context.Background()

	// drain burst
	for i := 0; i < 2; i++ {
		if err := l.Wait(ctx); err != nil {
			t.Fatalf("burst wait %d failed: %v", i+1, err)
		}
	}

	// next should take ~500ms
	start := time.Now()
	if err := l.Wait(ctx); err != nil {
		t.Fatalf("post-burst wait failed: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 400*time.Millisecond {
		t.Errorf("expected ≥400ms for next wait after burst, got %v", elapsed)
	}
}

// --- SlidingWindowLimiter tests ---

func TestSlidingWindow_RateWithinWindow(t *testing.T) {
	// 5 RPS, 1-second window: in any 1-second slice, at most 5 grants.
	// We measure over 2 seconds and expect ~10 total (±4 for timing).
	l := NewSlidingWindow(5, 1)
	defer l.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	count := 0
	for {
		if err := l.Wait(ctx); err != nil {
			break
		}
		count++
	}

	// Expect 6–14 grants (5 RPS × 2s = 10, ±40% tolerance for CI timing)
	if count < 6 || count > 14 {
		t.Errorf("sliding window 5 RPS/1s over 2s: got %d, want ~10", count)
	}
}

// --- BurstQuerier tests ---

func TestTokenBucket_TokensAvailable_InitialBurst(t *testing.T) {
	l := NewTokenBucket(10, 5)
	defer l.Stop()

	avail := l.TokensAvailable()
	if avail < 4 || avail > 5 {
		t.Errorf("initial tokens: got %d, want ~5", avail)
	}
}

func TestTokenBucket_TokensAvailable_DropsAfterConsumption(t *testing.T) {
	l := NewTokenBucket(1, 5)
	defer l.Stop()

	ctx := context.Background()
	// Consume 3 tokens.
	for i := 0; i < 3; i++ {
		if err := l.Wait(ctx); err != nil {
			t.Fatal(err)
		}
	}

	avail := l.TokensAvailable()
	if avail > 2 {
		t.Errorf("after consuming 3 of 5: got %d available, want ≤2", avail)
	}
}

// --- Factory tests ---

func TestNew_StrictAlgorithm(t *testing.T) {
	l, err := New("strict", 10, "rps", LimiterOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer l.Stop()
	if _, ok := l.(*StrictLimiter); !ok {
		t.Errorf("expected *StrictLimiter, got %T", l)
	}
}

func TestNew_TokenBucketAlgorithm(t *testing.T) {
	l, err := New("token_bucket", 10, "rps", LimiterOptions{BurstSize: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer l.Stop()
	if _, ok := l.(*TokenBucketLimiter); !ok {
		t.Errorf("expected *TokenBucketLimiter, got %T", l)
	}
}

func TestNew_SlidingWindowAlgorithm(t *testing.T) {
	l, err := New("sliding_window", 10, "rps", LimiterOptions{WindowSeconds: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer l.Stop()
	if _, ok := l.(*SlidingWindowLimiter); !ok {
		t.Errorf("expected *SlidingWindowLimiter, got %T", l)
	}
}

func TestNew_RPMUnit(t *testing.T) {
	// 60 RPM = 1 RPS
	l, err := New("strict", 60, "rpm", LimiterOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer l.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()

	// Should grant within one second
	start := time.Now()
	if err := l.Wait(ctx); err != nil {
		t.Fatalf("wait failed: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed > 1200*time.Millisecond {
		t.Errorf("first wait took %v, expected ≤1200ms for 60 RPM", elapsed)
	}
}

func TestNew_UnknownAlgorithm(t *testing.T) {
	_, err := New("unknown", 10, "rps", LimiterOptions{})
	if err == nil {
		t.Fatal("expected error for unknown algorithm, got nil")
	}
}

func TestNew_ZeroRate(t *testing.T) {
	_, err := New("strict", 0, "rps", LimiterOptions{})
	if err == nil {
		t.Fatal("expected error for zero rate, got nil")
	}
}
