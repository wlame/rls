#!/usr/bin/env bash
# Token window: 100 tokens per 5s window, best-fit scheduling.
# Scenario 1: 7 requests cost=40 + 20 requests cost=3 — multi-window scheduling.
# Scenario 2: impossible request (cost=150 > capacity=100) → immediate 429.
# Scenario 3: admission timeout with ?timeout=1 when queue is loaded → 429.
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
        echo "  $(now)  ← $label  HTTP $status  $(echo "$body" | jq -r '"endpoint=\(.endpoint)  waited=\(.queued_for_ms)ms  tokens_consumed=\(.tokens_consumed)  remaining=\(.tokens_remaining)  capacity=\(.window_capacity)  waiting=\(.waiting_for_next_window)"')"
    fi
}

# --- Scenario 1: multi-window scheduling ---
echo "$(now)  Scenario 1: 7×cost=40 + 20×cost=3 to /tokens (100 tokens/5s window)"
echo "           Best-fit: ~2×40 + 6×3 = 98 per window, ~4 windows total."
echo ""

for i in $(seq 1 7); do
    (
        echo "  $(now)  → [L$i] GET /tokens?tokens=40"
        status=$(curl -sf -o /tmp/tw_resp_L$i -w '%{http_code}' "http://$HOST/tokens?tokens=40&req=L$i" || true)
        body=$(cat /tmp/tw_resp_L$i 2>/dev/null || echo '{}')
        recv "[L$i]" "$status" "$body"
    ) &
done

for i in $(seq 1 20); do
    (
        echo "  $(now)  → [S$i] GET /tokens?tokens=3"
        status=$(curl -sf -o /tmp/tw_resp_S$i -w '%{http_code}' "http://$HOST/tokens?tokens=3&req=S$i" || true)
        body=$(cat /tmp/tw_resp_S$i 2>/dev/null || echo '{}')
        recv "[S$i]" "$status" "$body"
    ) &
done
wait

echo ""
echo "$(now)  All 27 requests served. Check window distribution in server log."

# --- Scenario 2: impossible request ---
echo ""
echo "$(now)  Scenario 2: cost=150 > capacity=100 → immediate 429"
(
    echo "  $(now)  → [impossible] GET /tokens?tokens=150"
    status=$(curl -sf -o /tmp/tw_resp_impossible -w '%{http_code}' "http://$HOST/tokens?tokens=150" || true)
    body=$(cat /tmp/tw_resp_impossible 2>/dev/null || echo '{}')
    recv "[impossible]" "$status" "$body"
) &
wait

# --- Scenario 3: admission timeout ---
echo ""
echo "$(now)  Scenario 3: fill queue, then ?timeout=1 → 429 (estimated wait > 1s)"

# Queue up some large requests to inflate pendingTokens.
for i in $(seq 1 5); do
    (
        curl -sf -o /dev/null "http://$HOST/tokens?tokens=40&bg=$i" || true
    ) &
done
sleep 0.5

(
    echo "  $(now)  → [timeout] GET /tokens?tokens=40&timeout=1"
    status=$(curl -sf -o /tmp/tw_resp_timeout -w '%{http_code}' "http://$HOST/tokens?tokens=40&timeout=1" || true)
    body=$(cat /tmp/tw_resp_timeout 2>/dev/null || echo '{}')
    recv "[timeout]" "$status" "$body"
) &
wait

echo "  (done)"
