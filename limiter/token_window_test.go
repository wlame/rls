package limiter

import (
	"testing"
	"time"
)

func TestTokenWindow_TryConsume_WithinCapacity(t *testing.T) {
	tw := NewTokenWindow(100, time.Hour) // long window, won't reset during test
	defer tw.Stop()

	if !tw.TryConsume(40) {
		t.Error("TryConsume(40) should succeed with 100 remaining")
	}
	if tw.Remaining() != 60 {
		t.Errorf("remaining: got %d, want 60", tw.Remaining())
	}
}

func TestTokenWindow_TryConsume_ExactCapacity(t *testing.T) {
	tw := NewTokenWindow(50, time.Hour)
	defer tw.Stop()

	if !tw.TryConsume(50) {
		t.Error("TryConsume(50) should succeed with 50 remaining")
	}
	if tw.Remaining() != 0 {
		t.Errorf("remaining: got %d, want 0", tw.Remaining())
	}
}

func TestTokenWindow_TryConsume_ExceedsRemaining(t *testing.T) {
	tw := NewTokenWindow(100, time.Hour)
	defer tw.Stop()

	tw.TryConsume(80) // 20 left
	if tw.TryConsume(30) {
		t.Error("TryConsume(30) should fail with 20 remaining")
	}
	if tw.Remaining() != 20 {
		t.Errorf("remaining should be unchanged: got %d, want 20", tw.Remaining())
	}
}

func TestTokenWindow_Reset_RestoresCapacity(t *testing.T) {
	tw := NewTokenWindow(100, time.Hour)
	defer tw.Stop()

	tw.TryConsume(80)
	if tw.Remaining() != 20 {
		t.Fatalf("before reset: got %d, want 20", tw.Remaining())
	}

	tw.Reset()
	if tw.Remaining() != 100 {
		t.Errorf("after reset: got %d, want 100", tw.Remaining())
	}
}

func TestTokenWindow_Reset_SignalsResetCh(t *testing.T) {
	tw := NewTokenWindow(100, time.Hour)
	defer tw.Stop()

	tw.Reset()

	select {
	case <-tw.ResetCh():
		// expected
	case <-time.After(100 * time.Millisecond):
		t.Error("expected signal on ResetCh after Reset()")
	}
}

func TestTokenWindow_MultipleTryConsume_Accumulate(t *testing.T) {
	tw := NewTokenWindow(100, time.Hour)
	defer tw.Stop()

	tw.TryConsume(30)
	tw.TryConsume(25)
	tw.TryConsume(10)
	if tw.Remaining() != 35 {
		t.Errorf("remaining: got %d, want 35 (100-30-25-10)", tw.Remaining())
	}
}

func TestTokenWindow_Capacity(t *testing.T) {
	tw := NewTokenWindow(42, time.Hour)
	defer tw.Stop()

	if tw.Capacity() != 42 {
		t.Errorf("capacity: got %d, want 42", tw.Capacity())
	}
	tw.TryConsume(10)
	if tw.Capacity() != 42 {
		t.Errorf("capacity should not change after consume: got %d", tw.Capacity())
	}
}

func TestTokenWindow_Stop_DoesNotPanic(t *testing.T) {
	tw := NewTokenWindow(100, time.Hour)
	tw.Stop()
	tw.Stop() // double-stop should not panic
}

func TestTokenWindow_TickerResetsCapacity(t *testing.T) {
	tw := NewTokenWindow(100, 50*time.Millisecond)
	defer tw.Stop()

	tw.TryConsume(100)
	if tw.Remaining() != 0 {
		t.Fatalf("after full consume: got %d, want 0", tw.Remaining())
	}

	// Wait for ticker to reset.
	select {
	case <-tw.ResetCh():
	case <-time.After(200 * time.Millisecond):
		t.Fatal("ticker did not fire within 200ms")
	}

	if tw.Remaining() != 100 {
		t.Errorf("after ticker reset: got %d, want 100", tw.Remaining())
	}
}
