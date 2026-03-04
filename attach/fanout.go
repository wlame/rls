package attach

import "github.com/wlame/rls/endpoint"

// Fanout2 splits one source channel into two buffered output channels.
// A goroutine reads from src and non-blocking-sends to both outputs.
// Both output channels are closed when src is closed.
func Fanout2[T any](src <-chan T, bufSize int) (<-chan T, <-chan T) {
	a := make(chan T, bufSize)
	b := make(chan T, bufSize)
	go func() {
		defer close(a)
		defer close(b)
		for v := range src {
			select {
			case a <- v:
			default:
			}
			select {
			case b <- v:
			default:
			}
		}
	}()
	return a, b
}

// Events2 splits an event channel into two (buffer 256).
func Events2(src <-chan endpoint.Event) (<-chan endpoint.Event, <-chan endpoint.Event) {
	return Fanout2(src, 256)
}

// Logs2 splits a log channel into two (buffer 256).
func Logs2(src <-chan string) (<-chan string, <-chan string) {
	return Fanout2(src, 256)
}
