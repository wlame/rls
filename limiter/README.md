# limiter

The `limiter` package controls *when* rate-limited slots become available. Each endpoint owns one limiter; the dispatcher calls `Wait()` before releasing a queued ticket.

## Interface

```go
type Limiter interface {
    Wait(ctx context.Context) error   // block until a slot is available
    Stop()                            // release resources
}
```

## Algorithms

### `strict` — StrictLimiter

Fires at exact intervals using `time.Ticker`. One request every `1/rate` seconds, no bursting.

- **Config**: `algorithm: strict`
- **Behavior**: perfectly even spacing between requests

### `token_bucket` — TokenBucketLimiter

Wraps `golang.org/x/time/rate`. Allows bursting up to `burst_size` tokens, then throttles to the steady-state rate.

- **Config**: `algorithm: token_bucket`, `burst_size: N`
- **Behavior**: first N requests pass immediately, then one per `1/rate` seconds
- **Implements `BurstQuerier`**: exposes `TokensAvailable()` for admission timeout estimation

### `sliding_window` — SlidingWindowLimiter

Tracks grant timestamps in a ring buffer. Allows up to `rate * window_seconds` requests per window. Grants are reclaimed as they age out of the window.

- **Config**: `algorithm: sliding_window`, `window_seconds: N`
- **Behavior**: budget of `rate * window` slots; once exhausted, new slots open as old ones expire

### `token_window` — TokenWindow

A fixed-capacity token budget that resets every `window_seconds`. Unlike the other algorithms, `TokenWindow` does **not** implement the `Limiter` interface — it is a capacity tracker used directly by the endpoint's `dispatchTokenWindow()` loop.

- **Config**: `algorithm: token_window`, `tokens_per_window: N`, `window_seconds: S`, `default_tokens: D`
- **Behavior**: each request carries a token cost (`?tokens=N`); requests that fit in the current window are served immediately, others are deferred to the next window (best-fit scheduling)
- **Methods**: `TryConsume(cost) bool`, `Reset()`, `Capacity() int`, `Remaining() int`, `ResetCh() <-chan struct{}`, `Stop()`
- **Internal ticker**: resets capacity and signals `ResetCh` (buffered channel, size 1) every window

```go
tw := limiter.NewTokenWindow(10000, 60*time.Second)
defer tw.Stop()

tw.TryConsume(500)         // true, 9500 remaining
tw.TryConsume(9600)        // false, still 9500 remaining
// ... ticker fires after 60s ...
tw.TryConsume(9600)        // true, 400 remaining
```

## BurstQuerier

Optional interface for limiters that expose available burst tokens:

```go
type BurstQuerier interface {
    TokensAvailable() int
}
```

Currently implemented by `TokenBucketLimiter`. Used by the endpoint's admission timeout logic to subtract available burst tokens from the estimated wait calculation.

## Configuration

```yaml
endpoints:
  - path: "/api"
    rate: 10                  # requests per unit
    unit: rps                 # rps | rpm
    algorithm: token_bucket   # strict | token_bucket | sliding_window | token_window
    burst_size: 20            # token_bucket only
    window_seconds: 60        # sliding_window and token_window

  - path: "/llm"
    algorithm: token_window
    tokens_per_window: 10000  # token budget per window
    window_seconds: 60        # window duration
    default_tokens: 1         # cost when client omits ?tokens=
```

## Units

The `unit` field controls interpretation of `rate`:
- `rps` — requests per second (default)
- `rpm` — requests per minute (internally converted to RPS via `rate / 60`)

## Factory

```go
l, err := limiter.New("token_bucket", 10, "rps", limiter.LimiterOptions{
    BurstSize: 20,
})
```
