#!/usr/bin/env bash
# Post-deploy smoke (doc_v2/15 §8 — gate sau deploy). Kiểm public endpoints qua
# nginx + (nếu chạy local) trạng thái health container. Exit !=0 nếu bất kỳ check fail.
#
# Dùng:  infra/scripts/verify.sh [BASE_URL]
#   BASE_URL mặc định https://localhost (self-signed → curl -k). Prod: https://domain.
set -uo pipefail

BASE_URL="${1:-https://localhost}"
CURL="curl -fsS -k --max-time 10"
fail=0

check() { # name expected_code url
  local name="$1" want="$2" url="$3"
  local got
  got=$($CURL -o /dev/null -w "%{http_code}" "$url" 2>/dev/null || true)
  if [[ "$got" == "$want" ]]; then
    echo "✓ $name ($got)"
  else
    echo "✗ $name: want $want got ${got:-000} ($url)"
    fail=1
  fi
}

echo "== HTTP endpoints =="
check "frontend /login"      200 "$BASE_URL/login"
check "api /api/v1/me (401)" 401 "$BASE_URL/api/v1/me"

# HTTP→HTTPS redirect (chỉ khi BASE_URL là https).
if [[ "$BASE_URL" == https://* ]]; then
  http_url="${BASE_URL/https:/http:}"
  code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 10 "$http_url/login" 2>/dev/null || true)
  if [[ "$code" == "301" || "$code" == "308" ]]; then echo "✓ http→https redirect ($code)"; else echo "✗ http→https redirect: got ${code:-000}"; fail=1; fi
fi

# Container health (chỉ khi chạy local có docker compose).
if command -v docker >/dev/null 2>&1; then
  echo "== Container health =="
  cd "$(dirname "$0")/../docker" 2>/dev/null || true
  unhealthy=$(docker compose -f docker-compose.yml -f docker-compose.prod.yml ps \
    --format '{{.Name}} {{.Health}}' 2>/dev/null | grep -E 'unhealthy|starting' || true)
  if [[ -n "$unhealthy" ]]; then echo "✗ container chưa healthy:"; echo "$unhealthy"; fail=1; else echo "✓ mọi container healthy"; fi
fi

[[ $fail -eq 0 ]] && echo "VERIFY OK" || echo "VERIFY FAILED"
exit $fail
