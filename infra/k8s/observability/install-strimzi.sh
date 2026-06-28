#!/usr/bin/env bash
# install-strimzi.sh — Phase III C5: Strimzi operator (Helm) + Kafka CR (KRaft) + topics.
# Idempotent. Operator watch namespace observability (Kafka + KafkaTopic CR ở đó).
set -euo pipefail

CLUSTER="${CLUSTER:-logmon}"
NS="${NS:-observability}"
STRIMZI_VERSION="${STRIMZI_VERSION:-1.1.0}"   # doc_v2 §7: Strimzi 1.0 (1.x line)
K8S="$(cd "$(dirname "$0")/.." && pwd)"

kubectl config use-context "k3d-${CLUSTER}" >/dev/null
kubectl get ns "$NS" >/dev/null 2>&1 || kubectl apply -f "$K8S/base/namespace.yaml"

echo "install-strimzi: helm repo update"
helm repo add strimzi https://strimzi.io/charts/ >/dev/null 2>&1 || true
helm repo update strimzi >/dev/null

echo "install-strimzi: helm upgrade --install strimzi-operator ($STRIMZI_VERSION)"
helm upgrade --install strimzi-operator strimzi/strimzi-kafka-operator \
  --version "$STRIMZI_VERSION" \
  --namespace "$NS" \
  --set watchNamespaces="{$NS}" \
  --wait --timeout 5m

echo "install-strimzi: apply Kafka CR + topics"
kubectl apply -f "$K8S/observability/kafka.yaml"

echo "install-strimzi: chờ Kafka Ready (tối đa 6 phút)..."
if kubectl wait kafka/logmon -n "$NS" --for=condition=Ready --timeout=360s; then
  echo "✓ Kafka Ready"
  kubectl get kafkatopic -n "$NS"
  exit 0
fi
echo "install-strimzi: timeout; kiểm: kubectl describe kafka logmon -n $NS"; exit 1
