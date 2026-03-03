#!/usr/bin/env bash
# 10 concurrent requests to /vip (2 RPS, priority scheduler).
# Sent with X-Priority: 1 10 3 7 5 2 8 4 6 9.
# Each background request is launched 5ms after the previous one so timestamps
# are distinct while all 10 still arrive well before the first tick.
# Expected serve order (desc): 10→9→8→7→6→5→4→3→2→1.
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

echo "$(now)  Firing 10 concurrent requests to /vip with mixed X-Priority values (5ms between launches)..."
echo "           Priorities sent: 1 10 3 7 5 2 8 4 6 9  (expect serve order: 10→9→8→7→6→5→4→3→2→1)"
for priority in 1 10 3 7 5 2 8 4 6 9; do
    (
        echo "  $(now)  → [p=$priority] GET /vip?p=$priority"
        resp=$(curl -sf -H "X-Priority: $priority" "http://$HOST/vip?p=$priority")
        recv "[p=$priority]" "$resp"
    ) &
    sleep 0.005
done
wait
