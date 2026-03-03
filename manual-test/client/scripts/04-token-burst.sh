#!/usr/bin/env bash
# 15 concurrent requests to /burst (3 RPS, token_bucket, burst_size=10).
# Each background request is launched 5ms after the previous one so timestamps
# are distinct while all 15 still arrive within ~75ms — well before the first
# post-burst throttle interval (~333ms).
# Token bucket pre-fills 10 tokens; first 10 served near-instantly.
# Remaining 5 throttled at 3 RPS (~333ms apart).
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

echo "$(now)  Firing 15 concurrent requests to /burst (burst_size=10, rate=3 RPS, 5ms between launches)..."
echo "           First 10 should be near-instant; last 5 will be throttled at ~333ms each."
for i in $(seq 1 15); do
    (
        echo "  $(now)  → [$i] GET /burst?req=$i"
        resp=$(curl -sf "http://$HOST/burst?req=$i")
        recv "[$i]" "$resp"
    ) &
    sleep 0.005
done
wait
echo "  (all 15 done)"
