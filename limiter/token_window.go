package limiter

import (
	"context"
	"sync"
	"time"
)

// TokenWindow tracks a fixed-capacity token budget that resets every windowSize.
// It does NOT implement the Limiter interface — it is a capacity tracker used by
// the token_window dispatch loop in the endpoint package.
type TokenWindow struct {
	capacity   int
	remaining  int
	windowSize time.Duration

	mu      sync.Mutex
	ticker  *time.Ticker
	resetCh chan struct{} // buffered 1; coalesces multiple resets
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewTokenWindow creates a TokenWindow with the given capacity and window duration,
// and starts the background ticker that resets capacity every window.
func NewTokenWindow(capacity int, windowSize time.Duration) *TokenWindow {
	ctx, cancel := context.WithCancel(context.Background())
	tw := &TokenWindow{
		capacity:   capacity,
		remaining:  capacity,
		windowSize: windowSize,
		ticker:     time.NewTicker(windowSize),
		resetCh:    make(chan struct{}, 1),
		ctx:        ctx,
		cancel:     cancel,
	}
	go tw.loop()
	return tw
}

func (tw *TokenWindow) loop() {
	for {
		select {
		case <-tw.ctx.Done():
			return
		case <-tw.ticker.C:
			tw.Reset()
		}
	}
}

// TryConsume attempts to subtract cost tokens from the current window.
// Returns true if successful, false if insufficient capacity remains.
func (tw *TokenWindow) TryConsume(cost int) bool {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	if cost <= tw.remaining {
		tw.remaining -= cost
		return true
	}
	return false
}

// Reset refills the window to full capacity and signals the dispatcher
// via ResetCh. Multiple resets between dispatcher reads are coalesced
// (buffered channel of size 1, non-blocking send).
func (tw *TokenWindow) Reset() {
	tw.mu.Lock()
	tw.remaining = tw.capacity
	tw.mu.Unlock()

	select {
	case tw.resetCh <- struct{}{}:
	default:
	}
}

// Capacity returns the total token budget per window.
func (tw *TokenWindow) Capacity() int {
	return tw.capacity
}

// Remaining returns the tokens left in the current window.
func (tw *TokenWindow) Remaining() int {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	return tw.remaining
}

// ResetCh returns a channel that fires when the window resets.
// The dispatcher selects on this alongside the work channel.
func (tw *TokenWindow) ResetCh() <-chan struct{} {
	return tw.resetCh
}

// Stop cancels the background ticker goroutine and releases resources.
// Safe to call multiple times.
func (tw *TokenWindow) Stop() {
	tw.cancel()
	tw.ticker.Stop()
}
