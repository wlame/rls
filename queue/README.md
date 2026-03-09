# queue

The `queue` package provides bounded, thread-safe scheduling strategies for holding request tickets until the rate limiter releases them.

## Interface

All schedulers implement the `Queue` interface:

```go
type Queue interface {
    Push(t *Ticket) bool                       // enqueue; returns false when full
    Pop() *Ticket                              // dequeue next ticket (nil if empty)
    PopWhere(fn func(t *Ticket) bool) []*Ticket // selective pop: remove all matching tickets
    Len() int                                  // current queue depth
}
```

`Ticket` is the per-request handle. Its `Release` channel is used by the dispatcher to unblock the waiting HTTP handler. The `Cost` field carries the token cost for `token_window` endpoints (0 for other algorithms).

### PopWhere

`PopWhere` scans the queue and removes all tickets for which the predicate returns `true`, returning them in a slice. The predicate runs **under the queue's mutex** — this is intentional because the `token_window` algorithm uses a side-effecting predicate (`TryConsume`) that must be atomic with the removal.

Scan order matches each queue type's natural serve order:
- **FIFO**: front-to-back (oldest first)
- **LIFO**: back-to-front (newest first)
- **Priority**: highest priority first (pops all, tests predicate, re-pushes non-matches)
- **Random**: arbitrary order

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
