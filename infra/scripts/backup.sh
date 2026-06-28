#!/usr/bin/env bash
# Backup Postgres (pg_dump -Fc, doc_v2/10 §4). Chạy qua container postgres đang
# chạy. Output ./backups (gitignored). Prod nối B2/S3 ở phần cuối (ADR-021).
#
# Dùng:  infra/scripts/backup.sh
#   ENV: POSTGRES_USER (logmon), POSTGRES_DB (logmon), BACKUP_DIR (./backups),
#        RETENTION_DAYS (30), B2_BUCKET (rỗng → bỏ qua upload cloud).
set -euo pipefail

PGUSER="${POSTGRES_USER:-logmon}"
PGDB="${POSTGRES_DB:-logmon}"
BACKUP_DIR="${BACKUP_DIR:-$(cd "$(dirname "$0")/../.." && pwd)/backups}"
RETENTION_DAYS="${RETENTION_DAYS:-30}"
mkdir -p "$BACKUP_DIR"

cd "$(dirname "$0")/../docker"
stamp=$(date +%Y%m%d-%H%M%S)
out="$BACKUP_DIR/pg-$PGDB-$stamp.dump"

echo "→ pg_dump $PGDB → $out"
docker compose -f docker-compose.yml -f docker-compose.prod.yml exec -T postgres \
  pg_dump -U "$PGUSER" -d "$PGDB" -Fc > "$out"
echo "✓ $(du -h "$out" | cut -f1) $out"

# Xoá backup cũ quá hạn.
find "$BACKUP_DIR" -name 'pg-*.dump' -mtime +"$RETENTION_DAYS" -delete 2>/dev/null || true

# Upload cloud (ADR-021) — chỉ khi cấu hình. Cần rclone + remote 'b2' đã set up.
if [[ -n "${B2_BUCKET:-}" ]] && command -v rclone >/dev/null 2>&1; then
  echo "→ upload b2:$B2_BUCKET"
  rclone copy "$out" "b2:$B2_BUCKET/postgres/" && echo "✓ uploaded"
fi

echo "BACKUP OK"
