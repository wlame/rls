package attach

import (
	"testing"
	"time"
)

func TestFanout2_BothReceiveAll(t *testing.T) {
	src := make(chan int, 10)
	a, b := Fanout2(src, 10)

	for i := 0; i < 5; i++ {
		src <- i
	}
	close(src)

	// Collect all from both.
	var gotA, gotB []int
	for v := range a {
		gotA = append(gotA, v)
	}
	for v := range b {
		gotB = append(gotB, v)
	}

	if len(gotA) != 5 {
		t.Errorf("a: got %d items, want 5", len(gotA))
	}
	if len(gotB) != 5 {
		t.Errorf("b: got %d items, want 5", len(gotB))
	}
}

func TestFanout2_SlowConsumerDoesNotBlock(t *testing.T) {
	src := make(chan int, 10)
	a, _ := Fanout2(src, 1) // b has tiny buffer

	// Send more than b's buffer — should not block.
	for i := 0; i < 5; i++ {
		src <- i
	}
	close(src)

	// a should still get all items (buffer=1 but we read promptly).
	count := 0
	timeout := time.After(time.Second)
	for {
		select {
		case _, ok := <-a:
			if !ok {
				goto done
			}
			count++
		case <-timeout:
			t.Fatal("timeout — slow consumer blocked the fanout")
		}
	}
done:
	if count == 0 {
		t.Error("a received no items")
	}
}
