# rls — Rate Limiter echo Service

A small, async Go HTTP service that acts as a rate-limiting gate for outgoing HTTP requests.

Instead of implementing rate limiting inside each client application, clients make a blocking HTTP call to rls before each outgoing request. rls queues the request and responds only when the configured rate allows. The calling application is blocked for exactly the right amount of time.

## Why not a proxy?

rls is **not** a proxy. It never sees your actual requests — no credentials, no request bodies, no response data passes through it. Your client calls rls, waits for the green light, then talks to the target service directly.

This is intentional. A pure timing gate means:

- **No business logic** — rls has zero knowledge of your services, APIs, or data formats
- **No credentials exposure** — API keys, tokens, and auth headers never touch rls
- **No data in transit** — request/response payloads go straight from client to target
- **No retries or timeouts** — that's your client's concern, not the rate limiter's
- **No caching or status code analysis** — rls doesn't interpret responses
- **No redirect logic** — rls doesn't follow or rewrite URLs
- **No WebSockets or long polling** — simple blocking HTTP, nothing more
- **No service-specific knowledge** — the same rls instance serves any number of unrelated services

This makes rls a **Rate Limit as-a-Service** — a single shared piece of infrastructure that any team, any language, any service can use without coupling to it. One rls instance can gate requests to GitHub, OpenAI, Slack, and your internal APIs simultaneously, each with its own rate and scheduling strategy.

## Why

- **Client-agnostic**: works with any language, any HTTP client
- **No client logic**: no tokens, no buckets, no clocks in your app — just a blocking GET
- **Per-endpoint config**: different rate limits for different third-party services
- **Honest response**: each response tells you how long you waited and how deep the queue is

## Install

```bash
go install github.com/wlame/rls@latest
```

Or build from source:

```bash
git clone https://github.com/wlame/rls
cd rls
go build -o rls .
```

## Usage

```bash
# Start with built-in defaults (1 RPS on /, port 8080)
./rls

# Override port and host
./rls --port=9090 --host=127.0.0.1

# Load a config file
./rls --config=rls.yml

# CLI flags override config file values
./rls --config=rls.yml --port=9090

# Start with interactive terminal UI (recommended for development)
./rls --config=rls.yml --interactive
```

## Interactive TUI (`--interactive`)

The `--interactive` flag starts a live terminal dashboard alongside the HTTP server.
Useful during development to watch queue depth, serve rate, and wait-time distributions
without tailing logs.

```
 rls  http://0.0.0.0:8080
 ▶ /      FIFO 1rps  │ ●●●●●●●● ●●           [8/20] │  served:    42
   /fast  FIFO 5rps  │ ●●                    [2/100] │  rejected:   3
   /vip  PRIOR 2rps  │ ●●●●                   [4/20] │  p50:    180ms
   /burst FIFO 3rps  │                        [0/20] │  p99:    920ms
                                                      │  last:     3ms
 q quit  r reset stats  p pause  ↑↓/jk select  space inject
```

**Layout:**
- **Left**: configured endpoints with scheduler and rate
- **Middle**: live queue dots per endpoint (green <500ms · yellow <2s · red >2s) with `[queued/max]` counter
- **Right**: statistics for the selected endpoint (served, rejected, p50/p99 wait times, last wait)

**Keybindings:**

| Key | Action |
|-----|--------|
| `q` / `Ctrl+C` | Quit |
| `↑` / `↓` / `j` / `k` | Select endpoint |
| `r` | Reset stats for selected endpoint |
| `p` | Pause / resume display |
| `Space` | Inject a test GET request to the selected endpoint |

**BEL**: when the selected endpoint serves a request its queue decreases, the terminal emits a BEL
character (`\a`) — audible feedback if your terminal supports it.

## Config file

```yaml
server:
  host: "0.0.0.0"
  port: 8080

# Defaults applied to any endpoint that omits a field
defaults:
  scheduler: fifo          # fifo | lifo | priority | random
  algorithm: strict        # strict | token_bucket | sliding_window
  unit: rps                # rps | rpm
  max_queue_size: 1000
  overflow: reject         # reject (429) | block (wait forever)
  max_dynamic_endpoints: 1000  # cap on auto-created dynamic endpoints

endpoints:
  # Root "/" is auto-created at 1 RPS if not specified
  - path: "/"
    rate: 1

  - path: "/github"
    rate: 10
    unit: rps
    scheduler: fifo
    algorithm: strict
    max_queue_size: 500
    overflow: reject

  - path: "/openai"
    rate: 3600
    unit: rpm
    scheduler: priority    # clients can pass X-Priority: <int>
    algorithm: token_bucket
    burst_size: 20

  - path: "/slack"
    rate: 1
    unit: rps
    scheduler: fifo
    algorithm: sliding_window
    window_seconds: 60
```

## Scheduling strategies

| Strategy   | Behavior |
|------------|----------|
| `fifo`     | First in, first out (default) |
| `lifo`     | Last in, first out — newest requests served first |
| `priority` | Higher `X-Priority` header value = served sooner; ties broken by arrival order |
| `random`   | Random selection from the queue |

## Rate limiting algorithms

| Algorithm        | Behavior |
|------------------|----------|
| `strict`         | Exact interval — one response every 1/rate seconds |
| `token_bucket`   | Allows bursting up to `burst_size`, then throttles to the target rate |
| `sliding_window` | Counts requests in the last `window_seconds`; accurate for RPM-style limits |

## Response format

Every successful request returns JSON:

```json
{
  "ok": true,
  "endpoint": "/github",
  "queued_for_ms": 347,
  "queue_depth": 2,
  "rate": 10,
  "unit": "rps",
  "scheduler": "fifo",
  "algorithm": "strict"
}
```

| Field           | Description |
|-----------------|-------------|
| `queued_for_ms` | How long this request waited in the queue |
| `queue_depth`   | Number of requests still queued at time of response |
| `rate`          | Configured rate for this endpoint |

When the queue is full (`overflow: reject`) or the estimated wait exceeds `queue_timeout`, rls returns HTTP 429:

```json
{"ok": false, "error": "queue full"}
{"ok": false, "error": "estimated wait exceeds timeout"}
```

### Dynamic endpoints (hierarchical paths)

Requests to unconfigured paths automatically create **dynamic endpoints** that inherit configuration from the nearest configured ancestor via parent-path walking.

```
Request to /api/v2/users
  1. No exact match → walk parents
  2. /api/v2 not configured → /api configured → use as parent
  3. Create /api/v2/users with /api's rate, scheduler, algorithm, etc.
```

Each dynamic endpoint gets its **own independent queue and limiter** — it does not share the parent's rate limit. This provides per-path visibility and independent statistics. Dynamic endpoints persist until restart and appear in the TUI and attach mode.

```yaml
defaults:
  max_dynamic_endpoints: 1000   # DoS protection cap (default 1000)

endpoints:
  - path: "/"
    rate: 1
  - path: "/api"
    rate: 10
  # Requests to /api/v2, /api/v2/users, etc. auto-create endpoints at 10 RPS
```

In the interactive TUI, configured endpoints appear in **bold bright white** and dynamic endpoints in a **dimmer style**, rendered as a tree:

```
▶ /              FIFO 1rps   │ ●●●         [3/60]
  └ api          FIFO 10rps  │             [0/500]
    └ v2/users   FIFO 10rps  │ ●           [1/500]
  └ dynamic      FIFO 1rps   │             [0/60]
```

### Admission timeout

Set `queue_timeout` (seconds) to reject requests upfront when the predicted wait exceeds the threshold, instead of waiting for the queue to fill:

```yaml
- path: "/api"
  rate: 1
  queue_timeout: 3   # reject if estimated wait > 3s
```

Clients can override per-request with the `?timeout=N` query parameter (e.g. `?timeout=999` to effectively disable). A value of `0` (default) disables the check entirely. The timeout prediction is skipped for `lifo` and `random` schedulers where wait time is unpredictable.

## Client example (Python)

```python
import requests

def rate_limited_get(url, rls_endpoint="/github"):
    # Block until rls allows the next request
    requests.get(f"http://localhost:8080{rls_endpoint}")
    # Now make the actual outgoing request
    return requests.get(url)
```

## Priority header

Pass `X-Priority: <int>` to request earlier serving (only with `scheduler: priority`):

```bash
curl -H "X-Priority: 10" http://localhost:8080/openai
```

## License

MIT
