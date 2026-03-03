#!/usr/bin/env bash
# 10 staggered requests to /lifo (2 RPS, lifo scheduler).
# Requests sent 50ms apart so arrival order is well-defined.
# All 10 arrive before the first tick (~500ms); LIFO then pops last-arrived first.
# Expected serve order: ~10→9→8→7→6→5→4→3→2→1 (reverse of send order).
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

echo "$(now)  Firing 10 staggered requests to /lifo (2 RPS, 50ms apart)..."
echo "           LIFO: last request to arrive is served first."
echo "           All 10 arrive in ~500ms, well before the first tick."
for i in $(seq 1 10); do
    (
        echo "  $(now)  → [$i] GET /lifo?req=$i"
        resp=$(curl -sf "http://$HOST/lifo?req=$i")
        recv "[$i]" "$resp"
    ) &
    sleep 0.05
done
wait
echo "  (all 10 done — serve order in server log should be ~10→9→8→7→6→5→4→3→2→1)"
