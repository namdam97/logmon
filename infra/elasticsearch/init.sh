#!/bin/sh
# es-init — bootstrap ILM policy + index template cho LogMon (doc_v2/03 §4).
# Idempotent: PUT ghi đè, chạy lại an toàn. Chạy một lần mỗi lần stack up.
set -eu

ES_URL="${ES_URL:-http://elasticsearch:9200}"
AUTH="elastic:${ELASTIC_PASSWORD}"

echo "es-init: PUT _ilm/policy/logmon-logs"
curl -fsS -u "$AUTH" -X PUT "$ES_URL/_ilm/policy/logmon-logs" \
  -H 'Content-Type: application/json' -d @/init/ilm-policy.json > /dev/null

echo "es-init: PUT _index_template/logmon-logs"
curl -fsS -u "$AUTH" -X PUT "$ES_URL/_index_template/logmon-logs" \
  -H 'Content-Type: application/json' -d @/init/index-template.json > /dev/null

echo "es-init: done"
