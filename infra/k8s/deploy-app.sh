#!/usr/bin/env bash
# deploy-app.sh — Phase III C1: deploy core app + stateful deps lên k3d.
#
# Idempotent. Secret sinh runtime từ env (fallback local-dev-insecure) → KHÔNG
# commit secret vào git. Image build local + import vào cluster (k3d image import)
# → imagePullPolicy:Never, không cần registry.
#
# Env override (production/staging): POSTGRES_PASSWORD, JWT_SECRET,
# ALERTMANAGER_WEBHOOK_TOKEN, ELASTIC_PASSWORD, NOTIFICATION_ENCRYPTION_KEY.
set -euo pipefail

CLUSTER="${CLUSTER:-logmon}"
NS="${NS:-logmon}"
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
K8S="$ROOT/infra/k8s"

# ── Secrets (fallback CHỈ cho local dev — staging/prod PHẢI set qua env) ────────
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-logmon}"
JWT_SECRET="${JWT_SECRET:-local-dev-insecure-change-me}"
ALERTMANAGER_WEBHOOK_TOKEN="${ALERTMANAGER_WEBHOOK_TOKEN:-local-dev-webhook-token}"
ELASTIC_PASSWORD="${ELASTIC_PASSWORD:-local-dev-insecure}"
DATABASE_URL="postgres://logmon:${POSTGRES_PASSWORD}@postgres:5432/logmon?sslmode=disable"

echo "deploy-app: context k3d-$CLUSTER"
kubectl config use-context "k3d-${CLUSTER}" >/dev/null
kubectl get ns "$NS" >/dev/null 2>&1 || kubectl apply -f "$K8S/base/namespace.yaml"

echo "deploy-app: Secret logmon-secrets (sinh từ env, không commit)"
kubectl create secret generic logmon-secrets -n "$NS" \
  --from-literal=POSTGRES_PASSWORD="$POSTGRES_PASSWORD" \
  --from-literal=DATABASE_URL="$DATABASE_URL" \
  --from-literal=JWT_SECRET="$JWT_SECRET" \
  --from-literal=ALERTMANAGER_WEBHOOK_TOKEN="$ALERTMANAGER_WEBHOOK_TOKEN" \
  --from-literal=ELASTICSEARCH_PASSWORD="$ELASTIC_PASSWORD" \
  --dry-run=client -o yaml | kubectl apply -f -

echo "deploy-app: ConfigMap db-migrations (từ backend/migrations)"
kubectl create configmap db-migrations -n "$NS" \
  --from-file="$ROOT/backend/migrations" \
  --dry-run=client -o yaml | kubectl apply -f -

echo "deploy-app: build images (userservice, frontend)"
docker build -t logmon/userservice:local "$ROOT/backend"
docker build -t logmon/frontend:local "$ROOT/frontend"

echo "deploy-app: import images vào cluster k3d"
k3d image import logmon/userservice:local logmon/frontend:local -c "$CLUSTER"

echo "deploy-app: apply config + stateful deps"
kubectl apply -f "$K8S/app/configmap.yaml"
kubectl apply -f "$K8S/app/postgres.yaml"
kubectl apply -f "$K8S/app/redis.yaml"

echo "deploy-app: chờ Postgres ready (migrate phụ thuộc)"
kubectl rollout status statefulset/postgres -n "$NS" --timeout=180s

echo "deploy-app: chạy migrate Job (xoá Job cũ — spec immutable)"
kubectl delete job db-migrate -n "$NS" --ignore-not-found >/dev/null
kubectl apply -f "$K8S/app/migrate-job.yaml"
kubectl wait --for=condition=complete job/db-migrate -n "$NS" --timeout=120s

echo "deploy-app: apply app + ingress"
kubectl apply -f "$K8S/app/userservice.yaml"
kubectl apply -f "$K8S/app/frontend.yaml"
kubectl apply -f "$K8S/app/ingress.yaml"

echo "deploy-app: chờ rollout app"
kubectl rollout status deploy/userservice -n "$NS" --timeout=180s
kubectl rollout status deploy/frontend -n "$NS" --timeout=180s

cat <<EOF

✓ Core app deployed.
  Verify: curl -H 'Host: logmon.local' http://127.0.0.1:8088/login
          curl -H 'Host: logmon.local' http://127.0.0.1:8088/api/v1/me   # 401 (chưa login)
  Pods:   make k8s-ps
EOF
