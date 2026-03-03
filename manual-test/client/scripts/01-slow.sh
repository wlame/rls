#!/usr/bin/env bash
# 10 serial requests to /fast (5 RPS = 200ms between ticks).
# Sending one every 150ms — faster than the tick interval.
# Expected: each request waits at most one tick (~200ms).
set -euo pipefail
HOST="${RLS_HOST:-localhost:8080}"

now() {
    local t=$EPOCHREALTIME
    local sec=${t%.*} frac=${t#*.}
    printf '%s.%03d' "$(printf '%(%Y-%m-%d %H:%M:%S)T' "$sec")" "$(( 10#${frac:0:3} ))"
}

recv() {
    local label=$1 body=$2
    echo "  $(now)  ← $label  $(echo "$body" | jq -r '"endpoint=\(.endpoint)  waited=\(.queued_for_ms)ms  queue=\(.queue_depth)"')"
}

echo "$(now)  Sending 10 serial requests to /fast (5 RPS) with 150ms gaps..."
for i in $(seq 1 10); do
    echo "  $(now)  → [$i] GET /fast?req=$i"
    resp=$(curl -sf "http://$HOST/fast?req=$i")
    recv "[$i]" "$resp"
    sleep 0.15
done
