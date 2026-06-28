#!/usr/bin/env bash
# install-eck.sh — Phase III C4: ECK operator (Helm) + Elasticsearch CR.
# Idempotent. Operator cài CRD (elasticsearch.k8s.elastic.co, …) + controller.
set -euo pipefail

CLUSTER="${CLUSTER:-logmon}"
NS="${NS:-observability}"
ECK_VERSION="${ECK_VERSION:-3.4.1}"   # doc_v2 §7: ECK 3.4
K8S="$(cd "$(dirname "$0")/.." && pwd)"

# sync_es_password: ECK sinh password user `elastic` ở secret logmon-es-elastic-user
# (ns observability). Đồng bộ sang Secret app `logmon-secrets` (ns logmon) để
# userservice (envFrom) đọc ELASTICSEARCH_PASSWORD, rồi restart để nạp env mới.
sync_es_password() {
  local pw
  pw=$(kubectl get secret logmon-es-elastic-user -n "$NS" -o jsonpath='{.data.elastic}' | base64 -d)
  if kubectl get secret logmon-secrets -n logmon >/dev/null 2>&1; then
    echo "install-eck: đồng bộ ELASTICSEARCH_PASSWORD → logmon-secrets + restart userservice"
    kubectl patch secret logmon-secrets -n logmon --type merge \
      -p "{\"data\":{\"ELASTICSEARCH_PASSWORD\":\"$(printf '%s' "$pw" | base64 -w0)\"}}"
    kubectl rollout restart deploy/userservice -n logmon >/dev/null 2>&1 || true
  else
    echo "install-eck: (logmon-secrets chưa có — chạy make k8s-app trước; bỏ qua sync)"
  fi
}

kubectl config use-context "k3d-${CLUSTER}" >/dev/null
kubectl get ns "$NS" >/dev/null 2>&1 || kubectl apply -f "$K8S/base/namespace.yaml"

echo "install-eck: helm repo update"
helm repo add elastic https://helm.elastic.co >/dev/null 2>&1 || true
helm repo update elastic >/dev/null

echo "install-eck: helm upgrade --install eck-operator ($ECK_VERSION)"
helm upgrade --install eck-operator elastic/eck-operator \
  --version "$ECK_VERSION" \
  --namespace "$NS" \
  --wait --timeout 5m

echo "install-eck: apply Elasticsearch CR"
kubectl apply -f "$K8S/observability/elasticsearch.yaml"

echo "install-eck: chờ ES health green/yellow (tối đa 5 phút)..."
for i in $(seq 1 60); do
  health=$(kubectl get elasticsearch logmon -n "$NS" -o jsonpath='{.status.health}' 2>/dev/null || true)
  phase=$(kubectl get elasticsearch logmon -n "$NS" -o jsonpath='{.status.phase}' 2>/dev/null || true)
  echo "  health=$health phase=$phase"
  case "$health" in
    green|yellow)
      echo "✓ Elasticsearch sẵn sàng (health=$health)"
      sync_es_password
      exit 0 ;;
  esac
  sleep 5
done
echo "install-eck: timeout chờ ES; kiểm: kubectl describe elasticsearch logmon -n $NS"; exit 1
