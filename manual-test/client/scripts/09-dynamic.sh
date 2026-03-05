#!/usr/bin/env bash
# Dynamic endpoint creation: hit unconfigured paths, observe independent endpoints.
# /dynamic/path/deep and /dynamic/other both inherit config from / (1 RPS).
set -euo pipefail
HOST="${RLS_HOST:-localhost:8080}"

now() {
    local t=$EPOCHREALTIME
    local sec=${t%.*} frac=${t#*.}
    printf '%(%H:%M:%S)T.%s' "$sec" "${frac:0:3}"
}

echo ""
echo "--- Step 1: Hit configured / first ---"
resp=$(curl -sf "http://$HOST/")
echo "  $(now)  /  →  $resp"

sleep 1

echo ""
echo "--- Step 2: Hit undeclared /dynamic/path/deep (creates dynamic endpoint) ---"
resp=$(curl -sf "http://$HOST/dynamic/path/deep")
echo "  $(now)  /dynamic/path/deep  →  $resp"

sleep 1

echo ""
echo "--- Step 3: Hit undeclared /dynamic/other (creates another dynamic endpoint) ---"
resp=$(curl -sf "http://$HOST/dynamic/other")
echo "  $(now)  /dynamic/other  →  $resp"

sleep 1

echo ""
echo "--- Step 4: Hit /dynamic/path/deep again (should reuse same endpoint) ---"
resp=$(curl -sf "http://$HOST/dynamic/path/deep")
echo "  $(now)  /dynamic/path/deep  →  $resp"

echo ""
echo "--- Dynamic endpoint test complete ---"
echo "Check the TUI or --attach output to verify:"
echo "  - /dynamic/path/deep and /dynamic/other appear as separate endpoints"
echo "  - Each has independent statistics (served count, queue depth)"
echo "  - They inherit config from / (1 RPS, fifo, strict)"
