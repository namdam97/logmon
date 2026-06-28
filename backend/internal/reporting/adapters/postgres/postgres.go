// Package postgres implement persistence reporting BC (report_schedules +
// export_jobs) qua pgx/v5. Mọi query parameterized; filter workspace_id.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/namdam97/logmon/backend/internal/reporting/domain"
	"github.com/namdam97/logmon/backend/internal/reporting/ports"
)

// ScheduleRepository lưu report_schedules.
type ScheduleRepository struct {
	pool *pgxpool.Pool
}

// ExportJobRepository lưu export_jobs + claim pending.
type ExportJobRepository struct {
	pool *pgxpool.Pool
}

// Verify compliance tại compile time.
var (
	_ ports.ScheduleRepository  = (*ScheduleRepository)(nil)
	_ ports.ScheduleReader      = (*ScheduleRepository)(nil)
	_ ports.ExportJobRepository = (*ExportJobRepository)(nil)
)

// NewScheduleRepository tạo repo schedule.
func NewScheduleRepository(pool *pgxpool.Pool) *ScheduleRepository {
	return &ScheduleRepository{pool: pool}
}

// NewExportJobRepository tạo repo export job.
func NewExportJobRepository(pool *pgxpool.Pool) *ExportJobRepository {
	return &ExportJobRepository{pool: pool}
}

const _scheduleCols = `id, workspace_id, report_type, cron_expression, timezone, format,
	recipients, COALESCE(channel_id::text, ''), enabled, last_run_at, created_at`

// Save chèn schedule mới.
func (r *ScheduleRepository) Save(ctx context.Context, s domain.ReportSchedule) error {
	const q = `INSERT INTO report_schedules
		(id, workspace_id, report_type, cron_expression, timezone, format, recipients, channel_id, enabled, last_run_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`
	_, err := r.pool.Exec(ctx, q, s.ID(), s.WorkspaceID(), s.ReportType().String(), s.CronExpr(),
		s.Timezone(), s.Format().String(), s.Recipients(), nullUUID(s.ChannelID()), s.Enabled(),
		s.LastRunAt(), s.CreatedAt())
	if err != nil {
		return fmt.Errorf("insert schedule: %w", err)
	}
	return nil
}

// Update cập nhật schedule (enabled + last_run_at là phần hay đổi).
func (r *ScheduleRepository) Update(ctx context.Context, s domain.ReportSchedule) error {
	const q = `UPDATE report_schedules SET report_type=$1, cron_expression=$2, timezone=$3,
		format=$4, recipients=$5, channel_id=$6, enabled=$7, last_run_at=$8
		WHERE workspace_id=$9 AND id=$10`
	tag, err := r.pool.Exec(ctx, q, s.ReportType().String(), s.CronExpr(), s.Timezone(),
		s.Format().String(), s.Recipients(), nullUUID(s.ChannelID()), s.Enabled(), s.LastRunAt(),
		s.WorkspaceID(), s.ID())
	if err != nil {
		return fmt.Errorf("update schedule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrReportScheduleNotFound
	}
	return nil
}

// Delete xóa schedule.
func (r *ScheduleRepository) Delete(ctx context.Context, workspaceID, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM report_schedules WHERE workspace_id=$1 AND id=$2`, workspaceID, id)
	if err != nil {
		return fmt.Errorf("delete schedule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrReportScheduleNotFound
	}
	return nil
}

// ByID lấy schedule theo id trong workspace.
func (r *ScheduleRepository) ByID(ctx context.Context, workspaceID, id string) (domain.ReportSchedule, error) {
	q := `SELECT ` + _scheduleCols + ` FROM report_schedules WHERE workspace_id=$1 AND id=$2`
	return scanSchedule(r.pool.QueryRow(ctx, q, workspaceID, id))
}

// ListByWorkspace liệt kê schedule của workspace.
func (r *ScheduleRepository) ListByWorkspace(ctx context.Context, workspaceID string) ([]domain.ReportSchedule, error) {
	q := `SELECT ` + _scheduleCols + ` FROM report_schedules WHERE workspace_id=$1 ORDER BY created_at`
	return r.querySchedules(ctx, q, workspaceID)
}

// ListEnabled liệt kê mọi schedule đang bật (cho scheduler runner).
func (r *ScheduleRepository) ListEnabled(ctx context.Context) ([]domain.ReportSchedule, error) {
	q := `SELECT ` + _scheduleCols + ` FROM report_schedules WHERE enabled=true`
	return r.querySchedules(ctx, q)
}

func (r *ScheduleRepository) querySchedules(ctx context.Context, q string, args ...any) ([]domain.ReportSchedule, error) {
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query schedules: %w", err)
	}
	defer rows.Close()
	var out []domain.ReportSchedule
	for rows.Next() {
		s, err := scanSchedule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate schedules: %w", err)
	}
	return out, nil
}

func scanSchedule(row pgx.Row) (domain.ReportSchedule, error) {
	var (
		id, wid, rt, cron, tz, format, channelID string
		recipients                               []string
		enabled                                  bool
		lastRunAt                                *time.Time
		createdAt                                time.Time
	)
	if err := row.Scan(&id, &wid, &rt, &cron, &tz, &format, &recipients, &channelID, &enabled, &lastRunAt, &createdAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ReportSchedule{}, domain.ErrReportScheduleNotFound
		}
		return domain.ReportSchedule{}, fmt.Errorf("scan schedule: %w", err)
	}
	reportType, err := domain.ParseReportType(rt)
	if err != nil {
		return domain.ReportSchedule{}, err
	}
	f, err := domain.ParseReportFormat(format)
	if err != nil {
		return domain.ReportSchedule{}, err
	}
	return domain.ReconstructSchedule(id, wid, reportType, cron, tz, f, recipients, channelID, enabled, lastRunAt, createdAt), nil
}

const _jobCols = `id, workspace_id, user_id, export_type, query_params, format, status,
	COALESCE(row_count,0), COALESCE(file_path,''), COALESCE(file_size_bytes,0), created_at, completed_at, expires_at`

// Save chèn export job mới.
func (r *ExportJobRepository) Save(ctx context.Context, j domain.ExportJob) error {
	params, err := json.Marshal(j.QueryParams())
	if err != nil {
		return fmt.Errorf("marshal query params: %w", err)
	}
	const q = `INSERT INTO export_jobs
		(id, workspace_id, user_id, export_type, query_params, format, status, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`
	_, err = r.pool.Exec(ctx, q, j.ID(), j.WorkspaceID(), j.UserID(), j.ExportType().String(),
		params, j.Format().String(), j.Status().String(), j.CreatedAt())
	if err != nil {
		return fmt.Errorf("insert export job: %w", err)
	}
	return nil
}

// Update cập nhật trạng thái + kết quả job.
func (r *ExportJobRepository) Update(ctx context.Context, j domain.ExportJob) error {
	const q = `UPDATE export_jobs SET status=$1, row_count=$2, file_path=$3, file_size_bytes=$4,
		completed_at=$5, expires_at=$6 WHERE workspace_id=$7 AND id=$8`
	tag, err := r.pool.Exec(ctx, q, j.Status().String(), j.RowCount(), nullStr(j.FilePath()),
		j.FileSizeBytes(), j.CompletedAt(), j.ExpiresAt(), j.WorkspaceID(), j.ID())
	if err != nil {
		return fmt.Errorf("update export job: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrExportJobNotFound
	}
	return nil
}

// ByID lấy job theo id trong workspace (poll trạng thái).
func (r *ExportJobRepository) ByID(ctx context.Context, workspaceID, id string) (domain.ExportJob, error) {
	q := `SELECT ` + _jobCols + ` FROM export_jobs WHERE workspace_id=$1 AND id=$2`
	return scanJob(r.pool.QueryRow(ctx, q, workspaceID, id))
}

// ClaimNextPending atomic chuyển 1 job pending → processing (SKIP LOCKED) và trả về.
func (r *ExportJobRepository) ClaimNextPending(ctx context.Context) (domain.ExportJob, bool, error) {
	q := `UPDATE export_jobs SET status='processing'
		WHERE id = (SELECT id FROM export_jobs WHERE status='pending'
			ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1)
		RETURNING ` + _jobCols
	job, err := scanJob(r.pool.QueryRow(ctx, q))
	if err != nil {
		if errors.Is(err, domain.ErrExportJobNotFound) {
			return domain.ExportJob{}, false, nil
		}
		return domain.ExportJob{}, false, err
	}
	return job, true, nil
}

func scanJob(row pgx.Row) (domain.ExportJob, error) {
	var (
		id, wid, uid, et, format, status string
		rawParams                        []byte
		rowCount, fileSize               int64
		filePath                         string
		createdAt                        time.Time
		completedAt, expiresAt           *time.Time
	)
	if err := row.Scan(&id, &wid, &uid, &et, &rawParams, &format, &status, &rowCount, &filePath, &fileSize, &createdAt, &completedAt, &expiresAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ExportJob{}, domain.ErrExportJobNotFound
		}
		return domain.ExportJob{}, fmt.Errorf("scan export job: %w", err)
	}
	exportType, err := domain.ParseExportType(et)
	if err != nil {
		return domain.ExportJob{}, err
	}
	f, err := domain.ParseReportFormat(format)
	if err != nil {
		return domain.ExportJob{}, err
	}
	st, err := domain.ParseExportStatus(status)
	if err != nil {
		return domain.ExportJob{}, err
	}
	var params map[string]any
	if len(rawParams) > 0 {
		if err := json.Unmarshal(rawParams, &params); err != nil {
			return domain.ExportJob{}, fmt.Errorf("unmarshal query params: %w", err)
		}
	}
	return domain.ReconstructJob(id, wid, uid, exportType, params, f, st, rowCount, filePath, fileSize, createdAt, completedAt, expiresAt), nil
}

func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nullUUID(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
