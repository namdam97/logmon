#!/usr/bin/env sh
# govulncheck backend với allowlist CÓ CHỦ ĐÍCH. Dùng chung bởi CI (.github/
# workflows/ci.yml) và git pre-push hook — allowlist sống ở MỘT nơi (DRY).
#
# GO-2026-5662: Stored XSS trong Prometheus *web UI* (web/ui). LogMon chỉ dùng
# promql/parser + model/rulefmt để validate rule, KHÔNG import/serve web UI nên
# không bị ảnh hưởng thực tế; chưa có bản vá (Fixed in: N/A) nên không bump được.
# Mọi vuln KHÁC vẫn làm fail. GOVULN_ALLOW: danh sách ID cách nhau bởi dấu cách.
set -e

ALLOW="${GOVULN_ALLOW:-GO-2026-5662}"
ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT/backend"

if ! command -v govulncheck >/dev/null 2>&1; then
  echo "installing govulncheck..."
  go install golang.org/x/vuln/cmd/govulncheck@latest
fi
GOVULN="$(command -v govulncheck 2>/dev/null || echo "$(go env GOPATH)/bin/govulncheck")"

out="$("$GOVULN" ./... 2>&1)" || true
echo "$out"

ids="$(printf '%s\n' "$out" | grep -oE 'GO-[0-9]{4}-[0-9]+' | sort -u)"
rc=0
for id in $ids; do
  case " $ALLOW " in
    *" $id "*) echo "allowlisted: $id" ;;
    *) echo "::error::Vulnerability chưa được allowlist: $id"; rc=1 ;;
  esac
done
exit $rc
