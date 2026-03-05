# queue

The `queue` package provides bounded, thread-safe scheduling strategies for holding request tickets until the rate limiter releases them.

## Interface

All schedulers implement the `Queue` interface:

```go
type Queue interface {
    Push(t *Ticket) bool   // enqueue; returns false when full
    Pop() *Ticket          // dequeue next ticket (nil if empty)
    Len() int              // current queue depth
}
```

`Ticket` is the per-request handle. Its `Release` channel is used by the dispatcher to unblock the waiting HTTP handler.

## Schedulers

| Scheduler | Type | Behavior |
|-----------|------|----------|
| `fifo` | `FIFOQueue` | First-in, first-out. Oldest request served first. Default strategy. |
| `lifo` | `LIFOQueue` | Last-in, first-out (stack). Newest request served first. Useful when freshness matters more than fairness. |
| `priority` | `PriorityQueue` | Highest `X-Priority` header value served first. Ties broken by arrival order (earlier wins). Uses `container/heap`. |
| `random` | `RandomQueue` | Picks a random ticket from the queue. Swap-and-shrink removal for O(1) pop. |

## Configuration

Set via the `scheduler` field in endpoint config (or `defaults`):

```yaml
defaults:
  scheduler: fifo

endpoints:
  - path: "/vip"
    scheduler: priority
```

## Capacity

All schedulers are bounded by `max_queue_size`. When full, `Push()` returns `false` and the endpoint returns HTTP 429 (with `overflow: reject`) or retries (with `overflow: block`).

## Concurrency

Every scheduler guards its internal slice/heap with a `sync.Mutex`. All operations are safe for concurrent use from multiple HTTP handler goroutines.

## Factory

Use `queue.New(scheduler, maxSize)` to create the right implementation:

```go
q, err := queue.New("fifo", 1000)
```
