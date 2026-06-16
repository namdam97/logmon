#!/usr/bin/env bash
# loadgen.sh — Sinh traffic giả liên tục đến demo-order service.
# GET (80%) và POST (20%) xen kẽ theo tỉ lệ 4:1.
# Nhấn Ctrl-C để dừng.
set -euo pipefail

TARGET_URL="${TARGET_URL:-http://localhost:8081}"
INTERVAL_SEC="${INTERVAL_SEC:-0.5}"

echo "Bắt đầu load gen → ${TARGET_URL} (interval=${INTERVAL_SEC}s)"

i=0
while true; do
    if (( i % 5 == 4 )); then
        # POST 1/5 lần = 20%
        curl -sf -X POST "${TARGET_URL}/api/v1/orders" \
            -H "Content-Type: application/json" \
            -d '{"item":"widget","quantity":1}' > /dev/null 2>&1 || true
    else
        # GET 4/5 lần = 80%
        curl -sf "${TARGET_URL}/api/v1/orders" > /dev/null 2>&1 || true
    fi
    i=$(( i + 1 ))
    sleep "${INTERVAL_SEC}"
done
