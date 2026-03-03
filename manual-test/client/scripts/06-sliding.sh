#!/usr/bin/env bash
# 12 concurrent requests to /slide (4 RPS, sliding_window, 2s window).
# Each background request is launched 5ms after the previous one so timestamps
# are distinct while all 12 still arrive within ~60ms — well inside the window.
# Window budget = 4 * 2 = 8 slots per 2-second window.
# First 8 served quickly as slots are available; last 4 must wait for
# the window to slide and free up expired slots.
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

echo "$(now)  Firing 12 concurrent requests to /slide (4 RPS, 2s window = 8 slots, 5ms between launches)..."
echo "           First 8 served fast (window budget); last 4 wait for window to slide."
for i in $(seq 1 12); do
    (
        echo "  $(now)  → [$i] GET /slide?req=$i"
        resp=$(curl -sf "http://$HOST/slide?req=$i")
        recv "[$i]" "$resp"
    ) &
    sleep 0.005
done
wait
echo "  (all 12 done)"
