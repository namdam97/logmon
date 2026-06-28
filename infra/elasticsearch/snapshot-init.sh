#!/bin/sh
# es-snapshot-init — Mode B (profile scale, doc_v2/03 §4 + ADR-021).
# Đăng ký snapshot repository S3 (SeaweedFS) + SLM policy chụp logs-* định kỳ.
#
# Đường FREE (license Basic): ILM lo hot/warm/delete, SLM lo backup→object store.
# `searchable_snapshot` (cold phase trong doc_v2) là tính năng Enterprise TRẢ PHÍ
# → KHÔNG dùng ở bản free; lộ trình nâng cấp: Enterprise license hoặc OpenSearch
# (searchable snapshot free). Quyết định: hội đồng 2026-06-28, 2 phiếu B.
#
# Idempotent: PUT ghi đè; execute snapshot 1 lần để verify end-to-end.
set -eu

ES_URL="${ES_URL:-http://elasticsearch:9200}"
AUTH="elastic:${ELASTIC_PASSWORD}"

echo "snapshot-init: PUT _snapshot/s3_repo (bucket es-snapshots @ SeaweedFS)"
# endpoint/protocol lấy từ elasticsearch.yml (s3.client.default.*); creds ở
# keystore. path_style_access bắt buộc cho S3-compatible không virtual-host.
curl -fsS -u "$AUTH" -X PUT "$ES_URL/_snapshot/s3_repo" \
  -H 'Content-Type: application/json' -d '{
    "type": "s3",
    "settings": { "bucket": "es-snapshots", "client": "default", "path_style_access": true }
  }' > /dev/null

echo "snapshot-init: POST _snapshot/s3_repo/_verify (kiểm tra kết nối S3)"
curl -fsS -u "$AUTH" -X POST "$ES_URL/_snapshot/s3_repo/_verify" > /dev/null

echo "snapshot-init: PUT _slm/policy/logs-daily"
curl -fsS -u "$AUTH" -X PUT "$ES_URL/_slm/policy/logs-daily" \
  -H 'Content-Type: application/json' -d '{
    "schedule": "0 30 1 * * ?",
    "name": "<logs-snap-{now/d}>",
    "repository": "s3_repo",
    "config": { "indices": ["logs-*"], "include_global_state": false },
    "retention": { "expire_after": "90d", "min_count": 5, "max_count": 50 }
  }' > /dev/null

echo "snapshot-init: execute 1 snapshot để verify (SLM _execute)"
SNAP=$(curl -fsS -u "$AUTH" -X POST "$ES_URL/_slm/policy/logs-daily/_execute" \
  | sed -n 's/.*"snapshot_name":"\([^"]*\)".*/\1/p')
echo "snapshot-init: snapshot=$SNAP — chờ hoàn tất..."

i=0
while [ "$i" -lt 30 ]; do
  # KHÔNG -f: ngay sau execute, GET snapshot có thể trả 404 (chưa xuất hiện) →
  # coi như "đang chạy", lặp tiếp thay vì in lỗi.
  STATE=$(curl -sS -u "$AUTH" "$ES_URL/_snapshot/s3_repo/$SNAP" \
    | sed -n 's/.*"state":"\([A-Z]*\)".*/\1/p' | head -1)
  case "$STATE" in
    SUCCESS) echo "snapshot-init: OK — $SNAP state=SUCCESS"; exit 0 ;;
    FAILED|PARTIAL) echo "snapshot-init: LỖI — $SNAP state=$STATE"; exit 1 ;;
  esac
  i=$((i + 1)); sleep 2
done
echo "snapshot-init: timeout chờ snapshot $SNAP"; exit 1
