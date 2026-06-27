// Package postgres implement ports persistence của logpipeline mgmt (pipeline_configs
// + dlq_entries) qua pgx/v5. Mọi query parameterized; filter workspace_id.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/namdam97/logmon/backend/internal/logpipeline/domain"
	"github.com/namdam97/logmon/backend/internal/logpipeline/ports"
)

// ConfigRepository lưu cấu hình pipeline (mode + ILM) per workspace.
type ConfigRepository struct {
	pool *pgxpool.Pool
}

// DLQRepository quản lý dlq_entries.
type DLQRepository struct {
	pool *pgxpool.Pool
}

// Verify compliance tại compile time.
var (
	_ ports.PipelineConfigRepository = (*ConfigRepository)(nil)
	_ ports.DLQRepository            = (*DLQRepository)(nil)
	_ ports.DLQReader                = (*DLQRepository)(nil)
)

// NewConfigRepository tạo repository config.
func NewConfigRepository(pool *pgxpool.Pool) *ConfigRepository { return &ConfigRepository{pool: pool} }

// NewDLQRepository tạo repository DLQ.
func NewDLQRepository(pool *pgxpool.Pool) *DLQRepository { return &DLQRepository{pool: pool} }

// Get trả cấu hình; domain.ErrPipelineConfigNotFound nếu chưa có.
func (r *ConfigRepository) Get(ctx context.Context, workspaceID string) (domain.PipelineConfig, error) {
	const q = `SELECT workspace_id, mode, ilm_hot_days, ilm_warm_days, ilm_delete_days, updated_at,
	                  COALESCE(updated_by, '')
	           FROM pipeline_configs WHERE workspace_id = $1`
	var (
		wid, mode, updatedBy string
		hot, warm, del       int
		updatedAt            time.Time
	)
	err := r.pool.QueryRow(ctx, q, workspaceID).Scan(&wid, &mode, &hot, &warm, &del, &updatedAt, &updatedBy)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.PipelineConfig{}, domain.ErrPipelineConfigNotFound
		}
		return domain.PipelineConfig{}, fmt.Errorf("get pipeline config: %w", err)
	}
	m, err := domain.ParseMode(mode)
	if err != nil {
		return domain.PipelineConfig{}, err
	}
	ilm := domain.ILMPolicy{HotDays: hot, WarmDays: warm, DeleteDays: del}
	return domain.ReconstructPipelineConfig(wid, m, ilm, updatedAt, updatedBy), nil
}

// Upsert chèn/cập nhật cấu hình (unique theo workspace_id). updated_by rỗng→NULL.
func (r *ConfigRepository) Upsert(ctx context.Context, c domain.PipelineConfig) error {
	const q = `INSERT INTO pipeline_configs
	               (workspace_id, mode, ilm_hot_days, ilm_warm_days, ilm_delete_days, updated_at, updated_by)
	           VALUES ($1, $2, $3, $4, $5, $6, $7)
	           ON CONFLICT (workspace_id) DO UPDATE SET
	               mode = EXCLUDED.mode,
	               ilm_hot_days = EXCLUDED.ilm_hot_days,
	               ilm_warm_days = EXCLUDED.ilm_warm_days,
	               ilm_delete_days = EXCLUDED.ilm_delete_days,
	               updated_at = EXCLUDED.updated_at,
	               updated_by = EXCLUDED.updated_by`
	ilm := c.ILM()
	var by *string
	if c.UpdatedBy() != "" {
		v := c.UpdatedBy()
		by = &v
	}
	_, err := r.pool.Exec(ctx, q, c.WorkspaceID(), c.Mode().String(),
		ilm.HotDays, ilm.WarmDays, ilm.DeleteDays, c.UpdatedAt(), by)
	if err != nil {
		return fmt.Errorf("upsert pipeline config: %w", err)
	}
	return nil
}

// ByID lấy DLQ entry trong workspace; domain.ErrDLQEntryNotFound nếu không có.
func (r *DLQRepository) ByID(ctx context.Context, workspaceID string, id int64) (domain.DLQEntry, error) {
	const q = `SELECT id, workspace_id, raw_message, error_reason, COALESCE(source_service,''),
	                  retry_count, status, created_at, retried_at
	           FROM dlq_entries WHERE workspace_id = $1 AND id = $2`
	row := r.pool.QueryRow(ctx, q, workspaceID, id)
	return scanDLQ(row)
}

// UpdateStatus cập nhật trạng thái + retry_count + retried_at của entry.
func (r *DLQRepository) UpdateStatus(ctx context.Context, workspaceID string, id int64, status domain.DLQStatus, retryCount int, retriedAt *time.Time) error {
	const q = `UPDATE dlq_entries SET status = $1, retry_count = $2, retried_at = $3
	           WHERE workspace_id = $4 AND id = $5`
	tag, err := r.pool.Exec(ctx, q, status.String(), retryCount, retriedAt, workspaceID, id)
	if err != nil {
		return fmt.Errorf("update dlq status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrDLQEntryNotFound
	}
	return nil
}

// List trả entries của workspace (statusFilter rỗng = mọi trạng thái).
func (r *DLQRepository) List(ctx context.Context, workspaceID, statusFilter string, limit int) ([]domain.DLQEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	const q = `SELECT id, workspace_id, raw_message, error_reason, COALESCE(source_service,''),
	                  retry_count, status, created_at, retried_at
	           FROM dlq_entries
	           WHERE workspace_id = $1 AND ($2 = '' OR status = $2)
	           ORDER BY created_at DESC
	           LIMIT $3`
	rows, err := r.pool.Query(ctx, q, workspaceID, statusFilter, limit)
	if err != nil {
		return nil, fmt.Errorf("list dlq: %w", err)
	}
	defer rows.Close()

	var out []domain.DLQEntry
	for rows.Next() {
		e, err := scanDLQ(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dlq: %w", err)
	}
	return out, nil
}

// CountByStatus trả map status→count cho workspace.
func (r *DLQRepository) CountByStatus(ctx context.Context, workspaceID string) (map[string]int, error) {
	const q = `SELECT status, count(*) FROM dlq_entries WHERE workspace_id = $1 GROUP BY status`
	rows, err := r.pool.Query(ctx, q, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("count dlq: %w", err)
	}
	defer rows.Close()

	out := map[string]int{}
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return nil, fmt.Errorf("scan dlq count: %w", err)
		}
		out[status] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dlq counts: %w", err)
	}
	return out, nil
}

func scanDLQ(row pgx.Row) (domain.DLQEntry, error) {
	var (
		id                            int64
		wid, raw, reason, svc, status string
		retryCount                    int
		createdAt                     time.Time
		retriedAt                     *time.Time
	)
	if err := row.Scan(&id, &wid, &raw, &reason, &svc, &retryCount, &status, &createdAt, &retriedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.DLQEntry{}, domain.ErrDLQEntryNotFound
		}
		return domain.DLQEntry{}, fmt.Errorf("scan dlq: %w", err)
	}
	st, err := domain.ParseDLQStatus(status)
	if err != nil {
		return domain.DLQEntry{}, err
	}
	return domain.ReconstructDLQEntry(id, wid, raw, reason, svc, retryCount, st, createdAt, retriedAt), nil
}
