package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/namdam97/logmon/backend/internal/alerting/domain"
	"github.com/namdam97/logmon/backend/internal/alerting/ports"
)

const uniqueViolationCode = "23505"

const ruleColumns = `id, workspace_id, name, expression, for_duration, severity, service,
	labels, annotations, enabled, sync_status, sync_error, created_at, updated_at`

// RuleRepository lưu trữ + đọc alert rule trên PostgreSQL.
type RuleRepository struct {
	pool *pgxpool.Pool
}

var (
	_ ports.RuleRepository       = (*RuleRepository)(nil)
	_ ports.RuleReader           = (*RuleRepository)(nil)
	_ ports.RuleSyncStatusWriter = (*RuleRepository)(nil)
)

// NewRuleRepository tạo repository với pool.
func NewRuleRepository(pool *pgxpool.Pool) *RuleRepository { return &RuleRepository{pool: pool} }

// Save chèn rule mới (trong tx của ctx). Vi phạm UNIQUE(ws,name) → ErrRuleNameTaken.
func (r *RuleRepository) Save(ctx context.Context, rule domain.AlertRule) error {
	labels, err := json.Marshal(rule.Labels())
	if err != nil {
		return fmt.Errorf("marshal labels: %w", err)
	}
	annotations, err := json.Marshal(rule.Annotations())
	if err != nil {
		return fmt.Errorf("marshal annotations: %w", err)
	}
	forDur := pgtype.Interval{Microseconds: rule.ForDuration().Microseconds(), Valid: true}

	const q = `INSERT INTO alert_rules
		(id, workspace_id, name, expression, for_duration, severity, service,
		 labels, annotations, enabled, sync_status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`
	_, err = dbFrom(ctx, r.pool).Exec(ctx, q,
		rule.ID().String(), rule.WorkspaceID(), rule.Name(), rule.Expression(), forDur,
		rule.Severity().String(), rule.Service(), string(labels), string(annotations),
		rule.IsEnabled(), string(rule.SyncStatus()), rule.CreatedAt(), rule.UpdatedAt())
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode {
			return domain.ErrRuleNameTaken
		}
		return fmt.Errorf("insert alert rule: %w", err)
	}
	return nil
}

// Update ghi đè rule theo id (trong tx của ctx). Vi phạm UNIQUE(ws,name) →
// ErrRuleNameTaken; không có dòng nào khớp id → ErrRuleNotFound.
func (r *RuleRepository) Update(ctx context.Context, rule domain.AlertRule) error {
	labels, err := json.Marshal(rule.Labels())
	if err != nil {
		return fmt.Errorf("marshal labels: %w", err)
	}
	annotations, err := json.Marshal(rule.Annotations())
	if err != nil {
		return fmt.Errorf("marshal annotations: %w", err)
	}
	forDur := pgtype.Interval{Microseconds: rule.ForDuration().Microseconds(), Valid: true}

	const q = `UPDATE alert_rules SET
		name = $2, expression = $3, for_duration = $4, severity = $5, service = $6,
		labels = $7, annotations = $8, enabled = $9, sync_status = $10,
		sync_error = $11, updated_at = $12
		WHERE id = $1`
	tag, err := dbFrom(ctx, r.pool).Exec(ctx, q,
		rule.ID().String(), rule.Name(), rule.Expression(), forDur,
		rule.Severity().String(), rule.Service(), string(labels), string(annotations),
		rule.IsEnabled(), string(rule.SyncStatus()), nullableSyncError(rule.SyncError()), rule.UpdatedAt())
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode {
			return domain.ErrRuleNameTaken
		}
		return fmt.Errorf("update alert rule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrRuleNotFound
	}
	return nil
}

// Delete xoá rule theo id (trong tx của ctx); ErrRuleNotFound nếu không có.
func (r *RuleRepository) Delete(ctx context.Context, id domain.RuleID) error {
	const q = `DELETE FROM alert_rules WHERE id = $1`
	tag, err := dbFrom(ctx, r.pool).Exec(ctx, q, id.String())
	if err != nil {
		return fmt.Errorf("delete alert rule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrRuleNotFound
	}
	return nil
}

// MarkSynced đánh dấu mọi rule đã đồng bộ thành công sang Prometheus (chỉ chạm
// dòng có trạng thái khác để tránh bump updated_at thừa ở happy path).
func (r *RuleRepository) MarkSynced(ctx context.Context, now time.Time) error {
	const q = `UPDATE alert_rules
		SET sync_status = $1, sync_error = NULL, updated_at = $2
		WHERE sync_status <> $1 OR sync_error IS NOT NULL`
	if _, err := dbFrom(ctx, r.pool).Exec(ctx, q, string(domain.SyncSynced), now); err != nil {
		return fmt.Errorf("mark synced: %w", err)
	}
	return nil
}

// MarkSyncError đánh dấu mọi rule sync lỗi kèm thông điệp (sync pipeline fail).
func (r *RuleRepository) MarkSyncError(ctx context.Context, message string, now time.Time) error {
	const q = `UPDATE alert_rules
		SET sync_status = $1, sync_error = $2, updated_at = $3
		WHERE sync_status <> $1 OR sync_error IS DISTINCT FROM $2`
	if _, err := dbFrom(ctx, r.pool).Exec(ctx, q, string(domain.SyncError), message, now); err != nil {
		return fmt.Errorf("mark sync error: %w", err)
	}
	return nil
}

// nullableSyncError map chuỗi rỗng → NULL để cột sync_error sạch khi không lỗi.
func nullableSyncError(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ExistsByName kiểm rule trùng tên trong workspace.
func (r *RuleRepository) ExistsByName(ctx context.Context, workspaceID, name string) (bool, error) {
	const q = `SELECT EXISTS(SELECT 1 FROM alert_rules WHERE workspace_id = $1 AND name = $2)`
	var exists bool
	if err := dbFrom(ctx, r.pool).QueryRow(ctx, q, workspaceID, name).Scan(&exists); err != nil {
		return false, fmt.Errorf("exists by name: %w", err)
	}
	return exists, nil
}

// ByID đọc rule theo id; ErrRuleNotFound nếu không có.
func (r *RuleRepository) ByID(ctx context.Context, id domain.RuleID) (domain.AlertRule, error) {
	const q = `SELECT ` + ruleColumns + ` FROM alert_rules WHERE id = $1`
	rule, err := scanRule(dbFrom(ctx, r.pool).QueryRow(ctx, q, id.String()))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AlertRule{}, domain.ErrRuleNotFound
	}
	return rule, err
}

// List đọc các rule của một workspace (sắp theo name).
func (r *RuleRepository) List(ctx context.Context, workspaceID string) ([]domain.AlertRule, error) {
	const q = `SELECT ` + ruleColumns + ` FROM alert_rules WHERE workspace_id = $1 ORDER BY name`
	return r.queryRules(ctx, q, workspaceID)
}

// ListAll đọc mọi rule (mọi workspace) — dùng render rule file.
func (r *RuleRepository) ListAll(ctx context.Context) ([]domain.AlertRule, error) {
	const q = `SELECT ` + ruleColumns + ` FROM alert_rules ORDER BY workspace_id, name`
	return r.queryRules(ctx, q)
}

func (r *RuleRepository) queryRules(ctx context.Context, q string, args ...any) ([]domain.AlertRule, error) {
	rows, err := dbFrom(ctx, r.pool).Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query rules: %w", err)
	}
	defer rows.Close()

	var rules []domain.AlertRule
	for rows.Next() {
		rule, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rules: %w", err)
	}
	return rules, nil
}

// scanRow là phần giao của pgx.Row và pgx.Rows cho Scan.
type scanRow interface {
	Scan(dest ...any) error
}

func scanRule(row scanRow) (domain.AlertRule, error) {
	var (
		rawID, workspaceID, name, expr, sevStr, service string
		forDur                                          pgtype.Interval
		labelsRaw, annotationsRaw                       []byte
		enabled                                         bool
		syncStatus                                      string
		syncErr                                         *string
		createdAt, updatedAt                            time.Time
	)
	if err := row.Scan(&rawID, &workspaceID, &name, &expr, &forDur, &sevStr, &service,
		&labelsRaw, &annotationsRaw, &enabled, &syncStatus, &syncErr, &createdAt, &updatedAt); err != nil {
		return domain.AlertRule{}, err
	}

	id, err := domain.NewRuleID(rawID)
	if err != nil {
		return domain.AlertRule{}, fmt.Errorf("reconstruct id: %w", err)
	}
	severity, err := domain.NewSeverity(sevStr)
	if err != nil {
		return domain.AlertRule{}, fmt.Errorf("reconstruct severity: %w", err)
	}
	labels, err := unmarshalMap(labelsRaw)
	if err != nil {
		return domain.AlertRule{}, fmt.Errorf("reconstruct labels: %w", err)
	}
	annotations, err := unmarshalMap(annotationsRaw)
	if err != nil {
		return domain.AlertRule{}, fmt.Errorf("reconstruct annotations: %w", err)
	}
	se := ""
	if syncErr != nil {
		se = *syncErr
	}

	return domain.Reconstruct(domain.ReconstructInput{
		ID:          id,
		WorkspaceID: workspaceID,
		Name:        name,
		Expression:  expr,
		Service:     service,
		ForDuration: intervalToDuration(forDur),
		Severity:    severity,
		Labels:      labels,
		Annotations: annotations,
		Enabled:     enabled,
		SyncStatus:  domain.SyncStatus(syncStatus),
		SyncError:   se,
		CreatedAt:   createdAt.UTC(),
		UpdatedAt:   updatedAt.UTC(),
	}), nil
}

func unmarshalMap(raw []byte) (map[string]string, error) {
	if len(raw) == 0 {
		return map[string]string{}, nil
	}
	m := map[string]string{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// intervalToDuration đổi pgtype.Interval → time.Duration (đủ cho `for` ≤ ngày).
func intervalToDuration(iv pgtype.Interval) time.Duration {
	return time.Duration(iv.Microseconds)*time.Microsecond +
		time.Duration(iv.Days)*24*time.Hour
}
