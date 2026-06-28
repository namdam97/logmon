// Package postgres implement ports.QuotaRepository (workspace_quotas) qua pgx/v5.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/namdam97/logmon/backend/internal/usage/domain"
	"github.com/namdam97/logmon/backend/internal/usage/ports"
)

// QuotaRepository lưu hạn mức per workspace.
type QuotaRepository struct {
	pool *pgxpool.Pool
}

var _ ports.QuotaRepository = (*QuotaRepository)(nil)

// NewQuotaRepository tạo repo.
func NewQuotaRepository(pool *pgxpool.Pool) *QuotaRepository {
	return &QuotaRepository{pool: pool}
}

// Get trả quota; domain.ErrQuotaNotFound nếu chưa cấu hình.
func (r *QuotaRepository) Get(ctx context.Context, workspaceID string) (domain.Quota, error) {
	const q = `SELECT workspace_id, max_ingestion_bytes_per_day, max_storage_bytes, retention_days, updated_at
	           FROM workspace_quotas WHERE workspace_id = $1`
	var (
		wid                 string
		maxIngest, maxStore int64
		retention           int
		updatedAt           time.Time
	)
	if err := r.pool.QueryRow(ctx, q, workspaceID).Scan(&wid, &maxIngest, &maxStore, &retention, &updatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Quota{}, domain.ErrQuotaNotFound
		}
		return domain.Quota{}, fmt.Errorf("get quota: %w", err)
	}
	return domain.ReconstructQuota(wid, maxIngest, maxStore, retention, updatedAt), nil
}

// Upsert chèn/cập nhật quota (unique theo workspace_id).
func (r *QuotaRepository) Upsert(ctx context.Context, q domain.Quota) error {
	const sql = `INSERT INTO workspace_quotas
	             (workspace_id, max_ingestion_bytes_per_day, max_storage_bytes, retention_days, updated_at)
	             VALUES ($1, $2, $3, $4, $5)
	             ON CONFLICT (workspace_id) DO UPDATE SET
	               max_ingestion_bytes_per_day = EXCLUDED.max_ingestion_bytes_per_day,
	               max_storage_bytes = EXCLUDED.max_storage_bytes,
	               retention_days = EXCLUDED.retention_days,
	               updated_at = EXCLUDED.updated_at`
	_, err := r.pool.Exec(ctx, sql, q.WorkspaceID(), q.MaxIngestionBytesPerDay(), q.MaxStorageBytes(), q.RetentionDays(), q.UpdatedAt())
	if err != nil {
		return fmt.Errorf("upsert quota: %w", err)
	}
	return nil
}
