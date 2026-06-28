#!/usr/bin/env bash
# bootstrap.sh — tạo cluster k3s LOCAL cho LogMon qua k3d (Phase III, doc_v2/10 §7).
#
# Vì sao k3d (không phải k3s trực tiếp)? k3s cần root để cài systemd service;
# k3d chạy k3s TRONG Docker → không cần sudo, dùng lại Docker daemon đang có.
# Cùng distro k3s (v1.31.x), cùng Traefik ingress + local-path StorageClass →
# production-like, chỉ khác cách bootstrap. Lên VPS/prod thật thì dùng k3s/k8s
# managed; manifests trong infra/k8s/ tái dùng nguyên vẹn.
#
# Idempotent: chạy lại khi cluster đã tồn tại sẽ bỏ qua bước create.
set -euo pipefail

CLUSTER="${CLUSTER:-logmon}"
# Ingress publish qua loopback (KHÔNG 0.0.0.0) — không lộ ra LAN khi dev local.
HTTP_PORT="${HTTP_PORT:-8088}"
HTTPS_PORT="${HTTPS_PORT:-8443}"

command -v k3d >/dev/null || { echo "THIẾU k3d — cài: curl -sSfL https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | bash"; exit 1; }
command -v kubectl >/dev/null || { echo "THIẾU kubectl"; exit 1; }

if k3d cluster list "$CLUSTER" >/dev/null 2>&1; then
  echo "bootstrap: cluster '$CLUSTER' đã tồn tại — bỏ qua create."
else
  echo "bootstrap: tạo cluster '$CLUSTER' (1 server, Traefik ingress, local-path SC)"
  # --disable metrics-server: tiny cluster, kube-prometheus-stack lo metrics.
  k3d cluster create "$CLUSTER" \
    --servers 1 \
    --port "127.0.0.1:${HTTP_PORT}:80@loadbalancer" \
    --port "127.0.0.1:${HTTPS_PORT}:443@loadbalancer" \
    --k3s-arg "--disable=metrics-server@server:0" \
    --wait --timeout 180s
fi

kubectl config use-context "k3d-${CLUSTER}" >/dev/null
echo "bootstrap: chờ kube-system core ready..."
kubectl wait --for=condition=Ready pod -l k8s-app=kube-dns -n kube-system --timeout=120s >/dev/null

echo "bootstrap: áp namespaces"
kubectl apply -f "$(dirname "$0")/base/namespace.yaml"

cat <<EOF

✓ Cluster '$CLUSTER' sẵn sàng.
  Ingress:   http://127.0.0.1:${HTTP_PORT}  |  https://127.0.0.1:${HTTPS_PORT}
  Context:   k3d-${CLUSTER}  (kubectl config use-context k3d-${CLUSTER})
  Tiếp theo: make k8s-app   (deploy core: PG/Redis/migrate/userservice/frontend/Ingress)
EOF
