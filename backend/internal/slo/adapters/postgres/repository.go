package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/namdam97/logmon/backend/internal/slo/domain"
	"github.com/namdam97/logmon/backend/internal/slo/ports"
)

const uniqueViolationCode = "23505"

const sloColumns = `id, workspace_id, name, service, sli_type, latency_threshold_ms,
	target, window_days, sync_status, sync_error, created_at, updated_at`

// SLORepository lưu trữ + đọc SLO trên PostgreSQL.
type SLORepository struct {
	pool *pgxpool.Pool
}

var (
	_ ports.SLORepository       = (*SLORepository)(nil)
	_ ports.SLOReader           = (*SLORepository)(nil)
	_ ports.SLOSyncStatusWriter = (*SLORepository)(nil)
)

// NewSLORepository tạo repository với pool.
func NewSLORepository(pool *pgxpool.Pool) *SLORepository { return &SLORepository{pool: pool} }

// Save chèn SLO mới (trong tx của ctx). Vi phạm UNIQUE(ws,name) → ErrSLONameTaken.
func (r *SLORepository) Save(ctx context.Context, s domain.SLO) error {
	const q = `INSERT INTO slos
		(id, workspace_id, name, service, sli_type, latency_threshold_ms,
		 target, window_days, sync_status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`
	_, err := dbFrom(ctx, r.pool).Exec(ctx, q,
		s.ID().String(), s.WorkspaceID(), s.Name(), s.Service(), s.SLIType().String(),
		nullableLatency(s.LatencyThresholdMs()), s.Target(), s.WindowDays(),
		string(s.SyncStatus()), s.CreatedAt(), s.UpdatedAt())
	if err != nil {
		if isUniqueViolation(err) {
			return domain.ErrSLONameTaken
		}
		return fmt.Errorf("insert slo: %w", err)
	}
	return nil
}

// Update ghi đè SLO theo id (trong tx của ctx).
func (r *SLORepository) Update(ctx context.Context, s domain.SLO) error {
	const q = `UPDATE slos SET
		name = $2, service = $3, sli_type = $4, latency_threshold_ms = $5,
		target = $6, window_days = $7, sync_status = $8, sync_error = $9, updated_at = $10
		WHERE id = $1`
	tag, err := dbFrom(ctx, r.pool).Exec(ctx, q,
		s.ID().String(), s.Name(), s.Service(), s.SLIType().String(),
		nullableLatency(s.LatencyThresholdMs()), s.Target(), s.WindowDays(),
		string(s.SyncStatus()), nullableSyncError(s.SyncErrorMessage()), s.UpdatedAt())
	if err != nil {
		if isUniqueViolation(err) {
			return domain.ErrSLONameTaken
		}
		return fmt.Errorf("update slo: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrSLONotFound
	}
	return nil
}

// Delete xoá SLO theo id (trong tx của ctx); ErrSLONotFound nếu không có.
func (r *SLORepository) Delete(ctx context.Context, id domain.SLOID) error {
	const q = `DELETE FROM slos WHERE id = $1`
	tag, err := dbFrom(ctx, r.pool).Exec(ctx, q, id.String())
	if err != nil {
		return fmt.Errorf("delete slo: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrSLONotFound
	}
	return nil
}

// MarkSynced đánh dấu mọi SLO đã đồng bộ rules thành công.
func (r *SLORepository) MarkSynced(ctx context.Context, now time.Time) error {
	const q = `UPDATE slos
		SET sync_status = $1, sync_error = NULL, updated_at = $2
		WHERE sync_status <> $1 OR sync_error IS NOT NULL`
	if _, err := dbFrom(ctx, r.pool).Exec(ctx, q, string(domain.SyncSynced), now); err != nil {
		return fmt.Errorf("mark synced: %w", err)
	}
	return nil
}

// MarkSyncError đánh dấu mọi SLO sync lỗi kèm thông điệp.
func (r *SLORepository) MarkSyncError(ctx context.Context, message string, now time.Time) error {
	const q = `UPDATE slos
		SET sync_status = $1, sync_error = $2, updated_at = $3
		WHERE sync_status <> $1 OR sync_error IS DISTINCT FROM $2`
	if _, err := dbFrom(ctx, r.pool).Exec(ctx, q, string(domain.SyncError), message, now); err != nil {
		return fmt.Errorf("mark sync error: %w", err)
	}
	return nil
}

// ExistsByName kiểm SLO trùng tên trong workspace.
func (r *SLORepository) ExistsByName(ctx context.Context, workspaceID, name string) (bool, error) {
	const q = `SELECT EXISTS(SELECT 1 FROM slos WHERE workspace_id = $1 AND name = $2)`
	var exists bool
	if err := dbFrom(ctx, r.pool).QueryRow(ctx, q, workspaceID, name).Scan(&exists); err != nil {
		return false, fmt.Errorf("exists by name: %w", err)
	}
	return exists, nil
}

// ByID đọc SLO theo id; ErrSLONotFound nếu không có.
func (r *SLORepository) ByID(ctx context.Context, id domain.SLOID) (domain.SLO, error) {
	const q = `SELECT ` + sloColumns + ` FROM slos WHERE id = $1`
	s, err := scanSLO(dbFrom(ctx, r.pool).QueryRow(ctx, q, id.String()))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SLO{}, domain.ErrSLONotFound
	}
	return s, err
}

// List đọc các SLO của một workspace (sắp theo name).
func (r *SLORepository) List(ctx context.Context, workspaceID string) ([]domain.SLO, error) {
	const q = `SELECT ` + sloColumns + ` FROM slos WHERE workspace_id = $1 ORDER BY name`
	return r.querySLOs(ctx, q, workspaceID)
}

// ListAll đọc mọi SLO (mọi workspace) — dùng render rule file.
func (r *SLORepository) ListAll(ctx context.Context) ([]domain.SLO, error) {
	const q = `SELECT ` + sloColumns + ` FROM slos ORDER BY workspace_id, name`
	return r.querySLOs(ctx, q)
}

func (r *SLORepository) querySLOs(ctx context.Context, q string, args ...any) ([]domain.SLO, error) {
	rows, err := dbFrom(ctx, r.pool).Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query slos: %w", err)
	}
	defer rows.Close()

	var slos []domain.SLO
	for rows.Next() {
		s, err := scanSLO(rows)
		if err != nil {
			return nil, err
		}
		slos = append(slos, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate slos: %w", err)
	}
	return slos, nil
}

// scanRow là phần giao của pgx.Row và pgx.Rows cho Scan.
type scanRow interface {
	Scan(dest ...any) error
}

func scanSLO(row scanRow) (domain.SLO, error) {
	var (
		rawID, workspaceID, name, service, sliStr string
		latencyMs                                 *int
		target                                    float64
		windowDays                                int
		syncStatus                                string
		syncErr                                   *string
		createdAt, updatedAt                      time.Time
	)
	if err := row.Scan(&rawID, &workspaceID, &name, &service, &sliStr, &latencyMs,
		&target, &windowDays, &syncStatus, &syncErr, &createdAt, &updatedAt); err != nil {
		return domain.SLO{}, err
	}

	id, err := domain.NewSLOID(rawID)
	if err != nil {
		return domain.SLO{}, fmt.Errorf("reconstruct id: %w", err)
	}
	sliType, err := domain.NewSLIType(sliStr)
	if err != nil {
		return domain.SLO{}, fmt.Errorf("reconstruct sli type: %w", err)
	}
	lat := 0
	if latencyMs != nil {
		lat = *latencyMs
	}
	se := ""
	if syncErr != nil {
		se = *syncErr
	}

	return domain.Reconstruct(domain.ReconstructInput{
		ID:          id,
		WorkspaceID: workspaceID,
		Name:        name,
		Service:     service,
		SLIType:     sliType,
		LatencyMs:   lat,
		Target:      target,
		WindowDays:  windowDays,
		SyncStatus:  domain.SyncStatus(syncStatus),
		SyncError:   se,
		CreatedAt:   createdAt.UTC(),
		UpdatedAt:   updatedAt.UTC(),
	}), nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode
}

// nullableLatency map 0 (availability) → NULL.
func nullableLatency(ms int) *int {
	if ms == 0 {
		return nil
	}
	return &ms
}

// nullableSyncError map chuỗi rỗng → NULL.
func nullableSyncError(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
