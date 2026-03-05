# endpoint

The `endpoint` package ties together a queue, a limiter, and an HTTP handler into a single rate-limited path. It is the core orchestrator of the rls request lifecycle.

## Architecture

Each `Endpoint` owns:
- A **Queue** (from the `queue` package) — holds waiting tickets
- A **Limiter** (from the `limiter` package) — controls release timing
- A **dispatcher goroutine** — pulls from the queue after acquiring a rate-limit slot
- A **work channel** — signals the dispatcher when a new ticket is pushed

### Request lifecycle

```
HTTP request → Handle()
  1. Create Ticket
  2. Admission timeout check (if configured)
  3. Push to Queue (or 429 if full)
  4. Signal work channel
  5. Block on Ticket.Release
  6. Dispatcher: wait for work → lim.Wait() → queue.Pop() → ticket.Release
  7. Return JSON response
```

## Admission Timeout

Predictive queue rejection based on estimated wait time. Configured via `queue_timeout` (seconds, 0 = disabled).

**How it works**: before pushing a ticket, the endpoint estimates how long the request would wait:
- `estimatedWait = queueLen / rps` (for fifo and priority schedulers)
- For `token_bucket`, available burst tokens are subtracted from the queue length
- For `lifo` and `random`, the check is skipped (wait time is unpredictable)

If the estimate exceeds the timeout, the request is rejected immediately with HTTP 429.

**Per-request override**: the `?timeout=N` query parameter (float seconds) overrides the endpoint config. Use `?timeout=999` to effectively disable the check for a single request.

## Overflow Modes

| Mode | Behavior |
|------|----------|
| `reject` | Return 429 immediately when queue is full (default) |
| `block` | Retry pushing until space opens or the server shuts down |

## Events

Endpoints optionally emit `Event` structs to a buffered channel for telemetry:

| Event | When |
|-------|------|
| `EventQueued` | Ticket successfully pushed to queue |
| `EventServed` | Ticket released and response sent |
| `EventRejected` | Request rejected (queue full or admission timeout) |

Configure with `WithEventSink(ch)`:
```go
ch := make(chan Event, 100)
ep, _ := endpoint.New(cfg, endpoint.WithEventSink(ch))
```

Events are non-blocking: if the channel is full, the event is dropped silently.

## Registry

`Registry` maps URL paths to endpoints. When a request arrives for an unconfigured path, a **dynamic endpoint** is created on the fly with its own independent queue and limiter, inheriting configuration from the nearest configured ancestor.

```go
reg, _ := endpoint.NewRegistryWithOpts(configs, []endpoint.RegistryOption{
    endpoint.WithMaxDynamic(1000),
}, endpoint.WithEventSink(ch))

ep, ok := reg.Match("/api/v2/users")  // creates dynamic endpoint inheriting from "/api"
```

### Dynamic endpoint creation

When `Match()` is called with a path that has no exact match in the registry:

1. Walk parent paths: `/api/v2/users` → `/api/v2` → `/api` → `/`
2. Find the nearest registered ancestor
3. Create a new endpoint with `config.InheritFrom()` — zero-value fields in the child are filled from the parent
4. Register it in the map with `Dynamic: true`

The dynamic endpoint gets its **own queue and limiter** — it does not share the parent's. This gives per-path visibility and independent statistics.

Dynamic endpoints persist until server restart. They appear in `Snapshot()`, `QueueDepths()`, TUI, and attach mode.

### DoS protection

Dynamic creation is capped by `max_dynamic_endpoints` (default 1000). Once the cap is reached, unconfigured paths fall back to the nearest configured parent's endpoint instead of creating a new one.

```yaml
defaults:
  max_dynamic_endpoints: 1000   # cap on dynamically created endpoints
```

### Snapshot

`Snapshot()` returns all endpoints (configured and dynamic) sorted by path:

```go
type EndpointInfo struct {
    Config   config.EndpointConfig
    QueueLen int
}

infos := reg.Snapshot()  // thread-safe, sorted by path
```

### Concurrency

The registry uses `sync.RWMutex`:
- `Match()` fast path (exact hit): `RLock` only
- `Match()` slow path (dynamic creation): releases `RLock`, acquires `Lock`, double-checks
- `Snapshot()`, `QueueDepths()`: `RLock`
- `StopAll()`: `Lock`

## Configuration Reference

```yaml
defaults:
  max_dynamic_endpoints: 1000  # cap on dynamically created endpoints (default 1000)

endpoints:
  - path: "/api"
    rate: 10                  # requests per unit
    unit: rps                 # rps | rpm
    scheduler: fifo           # fifo | lifo | priority | random
    algorithm: strict         # strict | token_bucket | sliding_window
    max_queue_size: 500       # max tickets in queue
    overflow: reject          # reject | block
    burst_size: 20            # token_bucket only
    window_seconds: 60        # sliding_window only
    queue_timeout: 3          # admission timeout in seconds (0 = disabled)
```

### Config inheritance

Dynamic endpoints inherit all zero-value fields from their nearest configured ancestor. The `InheritFrom(child, parent)` function fills these fields: `Rate`, `Unit`, `Scheduler`, `Algorithm`, `MaxQueueSize`, `Overflow`, `BurstSize`, `WindowSeconds`, `QueueTimeout`. The child's `Path` and `Dynamic` flag are always preserved.

## Response Format

Every response includes the full resolved configuration. For dynamic endpoints, inherited values are shown as if explicitly configured:

```json
{
  "ok": true,
  "endpoint": "/api/v2/users",
  "queued_for_ms": 347,
  "queue_depth": 2,
  "rate": 10,
  "unit": "rps",
  "scheduler": "fifo",
  "algorithm": "strict",
  "max_queue_size": 500,
  "overflow": "reject",
  "burst_size": 20,
  "queue_timeout": 3,
  "dynamic": true
}
```

Fields with zero values (`burst_size`, `window_seconds`, `queue_timeout`, `dynamic`) are omitted from JSON output.
