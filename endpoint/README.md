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

`Registry` maps URL paths to endpoints with longest-prefix matching:

```go
reg, _ := endpoint.NewRegistry(configs, endpoint.WithEventSink(ch))
ep, ok := reg.Match("/api/v2/users")  // matches "/api/v2" or "/api" or "/"
```

Matching priority: exact match > longest prefix > root `/` fallback.

## Configuration Reference

```yaml
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

## Response Format

```json
{
  "ok": true,
  "endpoint": "/api",
  "queued_for_ms": 347,
  "queue_depth": 2,
  "rate": 10,
  "unit": "rps",
  "scheduler": "fifo",
  "algorithm": "strict"
}
```
