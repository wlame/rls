package limiter

import (
	"testing"
	"time"
)

// --- StrictLimiter tests ---

func TestStrict_FiresAtApproximateRate(t *testing.T) {
	// 10 RPS → expect ~5 ticks in 500ms (±2 tolerance)
	l := NewStrict(10)
	defer l.Stop()

	count := 0
	deadline := time.After(500 * time.Millisecond)
loop:
	for {
		select {
		case <-l.Tick():
			count++
		case <-deadline:
			break loop
		}
	}

	// Expect 4–6 ticks (10 RPS × 0.5s = 5, ±20%)
	if count < 3 || count > 7 {
		t.Errorf("strict 10 RPS: got %d ticks in 500ms, want ~5", count)
	}
}

func TestStrict_Stop_DoesNotPanic(t *testing.T) {
	l := NewStrict(100)
	l.Stop()
	l.Stop() // double-stop should not panic
}

// --- TokenBucketLimiter tests ---

func TestTokenBucket_BurstReleasesUpToN(t *testing.T) {
	// burst=5 means up to 5 ticks available immediately.
	l := NewTokenBucket(100, 5)
	defer l.Stop()

	count := 0
	timeout := time.After(100 * time.Millisecond)
loop:
	for {
		select {
		case <-l.Tick():
			count++
		case <-timeout:
			break loop
		}
	}
	// With burst=5 and 100 RPS, 100ms window: expect 5+ ticks.
	if count < 5 {
		t.Errorf("token bucket burst=5: got %d ticks, want ≥5", count)
	}
}

func TestTokenBucket_ThrottlesAfterBurst(t *testing.T) {
	// 2 RPS, burst=2 → first 2 immediate, then ~1 per 500ms
	l := NewTokenBucket(2, 2)
	defer l.Stop()

	// drain burst
	for i := 0; i < 2; i++ {
		select {
		case <-l.Tick():
		case <-time.After(200 * time.Millisecond):
			t.Fatalf("burst tick %d timed out", i+1)
		}
	}

	// next tick should take ~500ms
	start := time.Now()
	select {
	case <-l.Tick():
		elapsed := time.Since(start)
		if elapsed < 400*time.Millisecond {
			t.Errorf("expected ≥400ms for next tick after burst, got %v", elapsed)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for post-burst tick")
	}
}

// --- SlidingWindowLimiter tests ---

func TestSlidingWindow_RateWithinWindow(t *testing.T) {
	// 5 RPS, 1-second window: in any 1-second slice, at most 5 ticks should fire.
	// We measure over 2 seconds and expect ~10 total (±4 for timing).
	l := NewSlidingWindow(5, 1)
	defer l.Stop()

	count := 0
	deadline := time.After(2 * time.Second)
loop:
	for {
		select {
		case <-l.Tick():
			count++
		case <-deadline:
			break loop
		}
	}

	// Expect 6–14 ticks (5 RPS × 2s = 10, ±40% tolerance for CI timing)
	if count < 6 || count > 14 {
		t.Errorf("sliding window 5 RPS/1s over 2s: got %d ticks, want ~10", count)
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

	// Should fire approximately once per second
	start := time.Now()
	<-l.Tick()
	elapsed := time.Since(start)
	if elapsed > 1200*time.Millisecond {
		t.Errorf("first tick took %v, expected ≤1200ms for 60 RPM", elapsed)
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
