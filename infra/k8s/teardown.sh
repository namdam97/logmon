#!/usr/bin/env bash
# teardown.sh — xoá cluster k3d LogMon (Phase III). Xoá cluster = xoá mọi PVC
# local-path → reset sạch. Để giữ data, dùng `k3d cluster stop logmon` thay vì script này.
set -euo pipefail
CLUSTER="${CLUSTER:-logmon}"
if k3d cluster list "$CLUSTER" >/dev/null 2>&1; then
  echo "teardown: xoá cluster '$CLUSTER' (mất toàn bộ PVC)..."
  k3d cluster delete "$CLUSTER"
  echo "✓ đã xoá."
else
  echo "teardown: cluster '$CLUSTER' không tồn tại — bỏ qua."
fi
