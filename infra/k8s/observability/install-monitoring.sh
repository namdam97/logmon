#!/usr/bin/env bash
# install-monitoring.sh — Phase III C2: kube-prometheus-stack qua Helm + ServiceMonitor.
# Idempotent (helm upgrade --install). CRD do chart cài (--wait). Pin chart version.
set -euo pipefail

CLUSTER="${CLUSTER:-logmon}"
NS="${NS:-observability}"
CHART_VERSION="${CHART_VERSION:-87.3.0}"   # doc_v2 §7 ghi 86.x; 87.x là GA hiện tại
K8S="$(cd "$(dirname "$0")/.." && pwd)"

kubectl config use-context "k3d-${CLUSTER}" >/dev/null
kubectl get ns "$NS" >/dev/null 2>&1 || kubectl apply -f "$K8S/base/namespace.yaml"

echo "install-monitoring: helm repo update"
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts >/dev/null 2>&1 || true
helm repo update prometheus-community >/dev/null

echo "install-monitoring: helm upgrade --install kps (chart $CHART_VERSION)"
helm upgrade --install kps prometheus-community/kube-prometheus-stack \
  --version "$CHART_VERSION" \
  --namespace "$NS" \
  -f "$K8S/observability/kube-prometheus-stack.values.yaml" \
  --wait --timeout 10m

echo "install-monitoring: ServiceMonitor LogMon"
kubectl apply -f "$K8S/observability/servicemonitor-logmon.yaml"

cat <<EOF

✓ kube-prometheus-stack deployed.
  Grafana:    http://127.0.0.1:8088/grafana/  (Host: logmon.local ; admin / admin-change-me)
  Prometheus: kubectl -n $NS port-forward svc/kps-prometheus 9090:9090
  Verify:     make k8s-ps
EOF
