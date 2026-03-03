#!/usr/bin/env bash
# 12 concurrent requests to /shuffle (2 RPS, random scheduler).
# Each background request is launched 5ms after the previous one so timestamps
# are distinct, but all arrive long before serving completes.
# All requests identical; serve order is randomised each run.
# Client output order reflects completion order (= random serve order).
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

echo "$(now)  Firing 12 concurrent requests to /shuffle (random serve order, 5ms between launches)..."
echo "           Send order is req=1..12; serve order will be scrambled."
for i in $(seq 1 12); do
    (
        echo "  $(now)  → [$i] GET /shuffle?req=$i"
        resp=$(curl -sf "http://$HOST/shuffle?req=$i")
        recv "[$i]" "$resp"
    ) &
    sleep 0.005
done
wait
