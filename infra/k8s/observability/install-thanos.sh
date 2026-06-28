#!/usr/bin/env bash
# install-thanos.sh — Phase III C7: SeaweedFS (S3) + Thanos (sidecar/store/query/compactor).
# Yêu cầu: C2 (kube-prometheus-stack) đã chạy. Idempotent.
set -euo pipefail

CLUSTER="${CLUSTER:-logmon}"
NS="${NS:-observability}"
CHART_VERSION="${CHART_VERSION:-87.3.0}"
K8S="$(cd "$(dirname "$0")/.." && pwd)"
OBS="$K8S/observability"

kubectl config use-context "k3d-${CLUSTER}" >/dev/null

echo "install-thanos: deploy SeaweedFS (object store)"
kubectl apply -f "$OBS/seaweedfs.yaml"
kubectl rollout status deploy/seaweedfs -n "$NS" --timeout=120s

echo "install-thanos: tạo bucket 'thanos' (S3 PUT, idempotent)"
kubectl run sw-mkbucket -n "$NS" --rm -i --restart=Never --image=curlimages/curl:8.14.1 --quiet -- \
  curl -s -o /dev/null -w "mkbucket HTTP %{http_code}\n" -X PUT "http://seaweedfs:8333/thanos" || true

echo "install-thanos: Secret thanos-objstore (objstore.yml)"
# In-cluster anonymous S3 → access/secret tuỳ ý; Thanos vẫn ký, SeaweedFS bỏ qua.
OBJSTORE=$(cat <<'YML'
type: S3
config:
  bucket: thanos
  endpoint: seaweedfs:8333
  insecure: true
  signature_version2: false
  access_key: thanos
  secret_key: thanos-local-insecure
YML
)
kubectl create secret generic thanos-objstore -n "$NS" \
  --from-literal=objstore.yml="$OBJSTORE" \
  --dry-run=client -o yaml | kubectl apply -f -

echo "install-thanos: helm upgrade kube-prometheus-stack (bật Thanos sidecar)"
helm upgrade kps prometheus-community/kube-prometheus-stack \
  --version "$CHART_VERSION" \
  --namespace "$NS" \
  -f "$OBS/kube-prometheus-stack.values.yaml" \
  --wait --timeout 10m

echo "install-thanos: deploy Thanos store/query/compactor"
kubectl apply -f "$OBS/thanos.yaml"
kubectl rollout status deploy/thanos-store -n "$NS" --timeout=120s
kubectl rollout status deploy/thanos-query -n "$NS" --timeout=120s
kubectl rollout status deploy/thanos-compactor -n "$NS" --timeout=120s

cat <<EOF

✓ Thanos deployed (SeaweedFS S3 + sidecar + store + query + compactor).
  Query UI: kubectl -n $NS port-forward svc/thanos-query 10902:10902 → http://127.0.0.1:10902
EOF
