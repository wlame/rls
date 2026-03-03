#!/usr/bin/env bash
# 12 concurrent requests to / (1 RPS, FIFO).
# Each background request is launched 5ms after the previous one — fast enough
# that all 12 are queued long before the first tick, but slow enough to give
# requests a defined arrival order visible in timestamps.
# Expected: queue starts at 11, waited time grows ~1s per position.
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

echo "$(now)  Firing 12 concurrent requests to / (1 RPS, 5ms between launches)..."
for i in $(seq 1 12); do
    (
        echo "  $(now)  → [$i] GET /?req=$i"
        resp=$(curl -sf "http://$HOST/?req=$i")
        recv "[$i]" "$resp"
    ) &
    sleep 0.005
done
wait
echo "  (all 12 done)"
