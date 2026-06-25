package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/namdam97/logmon/backend/internal/alerting/domain"
	"github.com/namdam97/logmon/backend/internal/alerting/ports"
)

const instanceColumns = `id, workspace_id, fingerprint, status, fired_at, resolved_at, labels`

// InstanceRepository lưu + đọc alert instance (nhận từ Alertmanager webhook).
type InstanceRepository struct {
	pool *pgxpool.Pool
}

var (
	_ ports.AlertInstanceRepository = (*InstanceRepository)(nil)
	_ ports.AlertInstanceReader     = (*InstanceRepository)(nil)
)

// NewInstanceRepository tạo repository với pool.
func NewInstanceRepository(pool *pgxpool.Pool) *InstanceRepository {
	return &InstanceRepository{pool: pool}
}

// UpsertFiring chèn instance firing, idempotent theo (fingerprint, fired_at):
// webhook lặp cho cùng một lần firing chỉ cập nhật labels/status (và bỏ resolved
// nếu alert tái firing trong cùng episode).
func (r *InstanceRepository) UpsertFiring(ctx context.Context, inst domain.AlertInstance) error {
	labels, err := json.Marshal(inst.Labels())
	if err != nil {
		return fmt.Errorf("marshal labels: %w", err)
	}
	const q = `INSERT INTO alert_instances
		(id, workspace_id, fingerprint, status, fired_at, labels)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (fingerprint, fired_at) DO UPDATE
		SET status = EXCLUDED.status, labels = EXCLUDED.labels, resolved_at = NULL`
	_, err = dbFrom(ctx, r.pool).Exec(ctx, q,
		inst.ID(), inst.WorkspaceID(), inst.Fingerprint().String(),
		string(inst.Status()), inst.FiredAt(), string(labels))
	if err != nil {
		return fmt.Errorf("upsert alert instance: %w", err)
	}
	return nil
}

// Resolve đánh dấu mọi instance đang mở của một fingerprint là resolved. No-op
// (không lỗi) nếu không có instance nào mở — webhook resolved có thể đến trùng.
func (r *InstanceRepository) Resolve(ctx context.Context, workspaceID, fingerprint string, at time.Time) error {
	const q = `UPDATE alert_instances
		SET status = $4, resolved_at = $3
		WHERE workspace_id = $1 AND fingerprint = $2 AND status <> $4`
	_, err := dbFrom(ctx, r.pool).Exec(ctx, q,
		workspaceID, fingerprint, at, string(domain.InstanceResolved))
	if err != nil {
		return fmt.Errorf("resolve alert instance: %w", err)
	}
	return nil
}

// ListActive trả về các instance chưa resolved của workspace (mới nhất trước).
func (r *InstanceRepository) ListActive(ctx context.Context, workspaceID string) ([]domain.AlertInstance, error) {
	const q = `SELECT ` + instanceColumns + `
		FROM alert_instances
		WHERE workspace_id = $1 AND status <> $2
		ORDER BY fired_at DESC`
	rows, err := dbFrom(ctx, r.pool).Query(ctx, q, workspaceID, string(domain.InstanceResolved))
	if err != nil {
		return nil, fmt.Errorf("query active instances: %w", err)
	}
	defer rows.Close()

	var out []domain.AlertInstance
	for rows.Next() {
		inst, err := scanInstance(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, inst)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate instances: %w", err)
	}
	return out, nil
}

func scanInstance(row scanRow) (domain.AlertInstance, error) {
	var (
		id, workspaceID, fpRaw, status string
		firedAt                        time.Time
		resolvedAt                     *time.Time
		labelsRaw                      []byte
	)
	if err := row.Scan(&id, &workspaceID, &fpRaw, &status, &firedAt, &resolvedAt, &labelsRaw); err != nil {
		return domain.AlertInstance{}, err
	}

	fp, err := domain.NewFingerprint(fpRaw)
	if err != nil {
		return domain.AlertInstance{}, fmt.Errorf("reconstruct fingerprint: %w", err)
	}
	labels, err := unmarshalMap(labelsRaw)
	if err != nil {
		return domain.AlertInstance{}, fmt.Errorf("reconstruct labels: %w", err)
	}
	resolved := time.Time{}
	if resolvedAt != nil {
		resolved = resolvedAt.UTC()
	}

	return domain.ReconstructInstance(domain.ReconstructInstanceInput{
		ID:          id,
		WorkspaceID: workspaceID,
		Fingerprint: fp,
		Status:      domain.InstanceStatus(status),
		FiredAt:     firedAt.UTC(),
		ResolvedAt:  resolved,
		Labels:      labels,
	}), nil
}
