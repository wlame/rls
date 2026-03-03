#!/usr/bin/env bash
set -euo pipefail
HOST="${RLS_HOST:-localhost:8080}"

banner() {
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  $1"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
}

# Wait for server to be ready
banner "Waiting for server at $HOST ..."
until curl -sf "http://$HOST/fast" > /dev/null 2>&1; do
    echo "  server not ready yet, retrying..."
    sleep 1
done
echo "  server is up!"

sleep 1

banner "01 — SERIAL to /fast (5 RPS) — 10 requests at 150ms gaps, all respond fast"
/scripts/01-slow.sh

sleep 3

banner "02 — BURST to / (1 RPS) — 12 concurrent, queue drains 1 per second"
/scripts/02-burst.sh

sleep 3

banner "03 — PRIORITY QUEUE to /vip (2 RPS) — 10 concurrent, high X-Priority served first"
/scripts/03-priority.sh

sleep 3

banner "04 — TOKEN BUCKET to /burst (3 RPS, burst=10) — 15 concurrent, first 10 instant"
/scripts/04-token-burst.sh

sleep 3

banner "05 — RANDOM ORDER to /shuffle (2 RPS) — 12 concurrent, unpredictable serve order"
/scripts/05-random.sh

sleep 3

banner "06 — SLIDING WINDOW to /slide (4 RPS, 2s window=8 slots) — 12 concurrent"
/scripts/06-sliding.sh

sleep 3

banner "07 — LIFO to /lifo (2 RPS) — 10 staggered, last arrived served first"
/scripts/07-lifo.sh

banner "All profiles complete."
