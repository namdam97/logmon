#!/usr/bin/env bash
# install-otel.sh — Phase III C6: OTel logs pipeline trên k8s (Mode B).
# agent (DaemonSet) → gateway (Deployment) → Kafka otlp_logs → consumer → ES.
# Yêu cầu: C4 (ES) + C5 (Kafka) đã chạy. Idempotent.
set -euo pipefail

CLUSTER="${CLUSTER:-logmon}"
NS="${NS:-observability}"
K8S="$(cd "$(dirname "$0")/.." && pwd)/observability"

kubectl config use-context "k3d-${CLUSTER}" >/dev/null

echo "install-otel: apply rbac + gateway + consumer + agent"
kubectl apply -f "$K8S/otel-rbac.yaml"
kubectl apply -f "$K8S/otel-gateway.yaml"
kubectl apply -f "$K8S/otel-consumer.yaml"
kubectl apply -f "$K8S/otel-agent.yaml"

echo "install-otel: chờ rollout"
kubectl rollout status deploy/otel-gateway -n "$NS" --timeout=120s
kubectl rollout status deploy/otel-consumer -n "$NS" --timeout=120s
kubectl rollout status daemonset/otel-agent -n "$NS" --timeout=120s

cat <<EOF

✓ OTel logs pipeline deployed.
  Flow: agent(DaemonSet) → gateway → Kafka otlp_logs → consumer → Elasticsearch
  Verify: kubectl exec -n $NS logmon-combined-0 -- bin/kafka-consumer-groups.sh \\
            --bootstrap-server localhost:9092 --describe --group otel-gateway
EOF
