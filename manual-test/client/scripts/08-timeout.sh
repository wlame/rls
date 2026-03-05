#!/usr/bin/env bash
# 8 concurrent requests to /timeout (1 RPS, queue_timeout=3s).
# Requests 1-3 accepted (estimated wait 0,1,2s ≤ 3s), 4+ rejected with 429.
# Then one request with ?timeout=999 → accepted despite queue depth.
set -euo pipefail
HOST="${RLS_HOST:-localhost:8080}"

now() {
    local t=$EPOCHREALTIME
    local sec=${t%.*} frac=${t#*.}
    printf '%s.%03d' "$(printf '%(%Y-%m-%d %H:%M:%S)T' "$sec")" "$(( 10#${frac:0:3} ))"
}

recv() {
    local label=$1 status=$2 body=$3
    if [ "$status" = "429" ]; then
        echo "  $(now)  ← $label  HTTP 429 REJECTED  $(echo "$body" | jq -r '.error // empty')"
    else
        echo "  $(now)  ← $label  HTTP $status  $(echo "$body" | jq -r '"endpoint=\(.endpoint)  waited=\(.queued_for_ms)ms  queue=\(.queue_depth)"')"
    fi
}

echo "$(now)  Firing 8 concurrent requests to /timeout (1 RPS, timeout=3s)..."
echo "           First ~3 accepted, rest rejected instantly with 429."
for i in $(seq 1 8); do
    (
        echo "  $(now)  → [$i] GET /timeout?req=$i"
        status=$(curl -sf -o /tmp/timeout_resp_$i -w '%{http_code}' "http://$HOST/timeout?req=$i" || true)
        body=$(cat /tmp/timeout_resp_$i 2>/dev/null || echo '{}')
        recv "[$i]" "$status" "$body"
    ) &
done
wait

echo ""
echo "$(now)  Now sending one request with ?timeout=999 (override) — should be accepted..."
(
    echo "  $(now)  → [override] GET /timeout?timeout=999"
    status=$(curl -sf -o /tmp/timeout_resp_override -w '%{http_code}' "http://$HOST/timeout?timeout=999" || true)
    body=$(cat /tmp/timeout_resp_override 2>/dev/null || echo '{}')
    recv "[override]" "$status" "$body"
) &
wait

echo "  (done — server log shows accepted vs rejected)"
