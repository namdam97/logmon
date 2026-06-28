#!/usr/bin/env bash
# Restore Postgres từ dump pg_dump -Fc (doc_v2/10 §4 — restore drill hàng quý).
# CẢNH BÁO: ghi đè dữ liệu hiện tại (--clean). Xác nhận trước khi chạy.
#
# Dùng:  infra/scripts/restore.sh <dump-file>
set -euo pipefail

DUMP="${1:?dùng: restore.sh <dump-file>}"
[[ -f "$DUMP" ]] || { echo "không thấy file: $DUMP"; exit 1; }
PGUSER="${POSTGRES_USER:-logmon}"
PGDB="${POSTGRES_DB:-logmon}"

read -r -p "Restore $DUMP vào DB '$PGDB' (GHI ĐÈ dữ liệu hiện tại)? [yes/N] " ans
[[ "$ans" == "yes" ]] || { echo "huỷ."; exit 1; }

cd "$(dirname "$0")/../docker"
echo "→ pg_restore --clean → $PGDB"
docker compose -f docker-compose.yml -f docker-compose.prod.yml exec -T postgres \
  pg_restore -U "$PGUSER" -d "$PGDB" --clean --if-exists --no-owner < "$DUMP"
echo "RESTORE OK — kiểm tra ứng dụng + chạy infra/scripts/verify.sh"
