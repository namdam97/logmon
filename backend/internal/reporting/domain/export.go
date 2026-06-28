package domain

import (
	"strings"
	"time"

	apperrors "github.com/namdam97/logmon/backend/internal/shared/errors"
)

// ExportType là loại dữ liệu xuất.
type ExportType int

// Enum bắt đầu từ 1.
const (
	// ExportLogs: xuất log.
	ExportLogs ExportType = iota + 1
	// ExportMetrics: xuất metrics.
	ExportMetrics
	// ExportReport: xuất báo cáo đã sinh.
	ExportReport
)

// String trả về biểu diễn khớp cột DB.
func (t ExportType) String() string {
	switch t {
	case ExportLogs:
		return "logs"
	case ExportMetrics:
		return "metrics"
	case ExportReport:
		return "report"
	default:
		return "unknown"
	}
}

// ParseExportType chuyển chuỗi thành ExportType.
func ParseExportType(raw string) (ExportType, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "logs":
		return ExportLogs, nil
	case "metrics":
		return ExportMetrics, nil
	case "report":
		return ExportReport, nil
	default:
		return 0, apperrors.NewValidationError("exportType", "must be logs|metrics|report")
	}
}

// ExportStatus là trạng thái vòng đời export job.
type ExportStatus int

const (
	// ExportPending: chờ worker xử lý.
	ExportPending ExportStatus = iota + 1
	// ExportProcessing: đang xử lý.
	ExportProcessing
	// ExportCompleted: hoàn tất, file sẵn sàng (signed URL).
	ExportCompleted
	// ExportFailed: thất bại.
	ExportFailed
)

// String trả về biểu diễn khớp cột DB.
func (s ExportStatus) String() string {
	switch s {
	case ExportPending:
		return "pending"
	case ExportProcessing:
		return "processing"
	case ExportCompleted:
		return "completed"
	case ExportFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// ParseExportStatus chuyển chuỗi thành ExportStatus.
func ParseExportStatus(raw string) (ExportStatus, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "pending":
		return ExportPending, nil
	case "processing":
		return ExportProcessing, nil
	case "completed":
		return ExportCompleted, nil
	case "failed":
		return ExportFailed, nil
	default:
		return 0, apperrors.NewValidationError("status", "invalid export status")
	}
}

// ExportJob là aggregate của một yêu cầu xuất dữ liệu bất đồng bộ. Bất biến —
// chuyển trạng thái trả bản sao mới có guard.
type ExportJob struct {
	id            string
	workspaceID   string
	userID        string
	exportType    ExportType
	queryParams   map[string]any
	format        ReportFormat
	status        ExportStatus
	rowCount      int64
	filePath      string
	fileSizeBytes int64
	createdAt     time.Time
	completedAt   *time.Time
	expiresAt     *time.Time
}

// NewJobInput là dữ liệu tạo export job.
type NewJobInput struct {
	ID          string
	WorkspaceID string
	UserID      string
	ExportType  ExportType
	QueryParams map[string]any
	Format      ReportFormat
	Now         time.Time
}

// NewExportJob tạo job mới (pending).
func NewExportJob(in NewJobInput) (ExportJob, error) {
	if strings.TrimSpace(in.ID) == "" {
		return ExportJob{}, apperrors.NewValidationError("id", "must not be empty")
	}
	if strings.TrimSpace(in.WorkspaceID) == "" {
		return ExportJob{}, apperrors.NewValidationError("workspaceId", "must not be empty")
	}
	if strings.TrimSpace(in.UserID) == "" {
		return ExportJob{}, apperrors.NewValidationError("userId", "must not be empty")
	}
	if !in.ExportType.valid() {
		return ExportJob{}, apperrors.NewValidationError("exportType", "invalid")
	}
	if !in.Format.valid() {
		return ExportJob{}, apperrors.NewValidationError("format", "invalid")
	}
	if in.Now.IsZero() {
		return ExportJob{}, apperrors.NewValidationError("createdAt", "must be set")
	}
	return ExportJob{
		id: in.ID, workspaceID: in.WorkspaceID, userID: in.UserID, exportType: in.ExportType,
		queryParams: copyParams(in.QueryParams), format: in.Format, status: ExportPending,
		createdAt: in.Now,
	}, nil
}

// ReconstructJob dựng lại từ storage — KHÔNG validate lại.
func ReconstructJob(id, workspaceID, userID string, et ExportType, params map[string]any, format ReportFormat, status ExportStatus, rowCount int64, filePath string, fileSizeBytes int64, createdAt time.Time, completedAt, expiresAt *time.Time) ExportJob {
	return ExportJob{
		id: id, workspaceID: workspaceID, userID: userID, exportType: et, queryParams: copyParams(params),
		format: format, status: status, rowCount: rowCount, filePath: filePath, fileSizeBytes: fileSizeBytes,
		createdAt: createdAt, completedAt: completedAt, expiresAt: expiresAt,
	}
}

// Accessors.
func (j ExportJob) ID() string                  { return j.id }
func (j ExportJob) WorkspaceID() string         { return j.workspaceID }
func (j ExportJob) UserID() string              { return j.userID }
func (j ExportJob) ExportType() ExportType      { return j.exportType }
func (j ExportJob) QueryParams() map[string]any { return copyParams(j.queryParams) }
func (j ExportJob) Format() ReportFormat        { return j.format }
func (j ExportJob) Status() ExportStatus        { return j.status }
func (j ExportJob) RowCount() int64             { return j.rowCount }
func (j ExportJob) FilePath() string            { return j.filePath }
func (j ExportJob) FileSizeBytes() int64        { return j.fileSizeBytes }
func (j ExportJob) CreatedAt() time.Time        { return j.createdAt }
func (j ExportJob) CompletedAt() *time.Time     { return j.completedAt }
func (j ExportJob) ExpiresAt() *time.Time       { return j.expiresAt }

// MarkProcessing chuyển pending→processing. Chỉ job pending mới process được.
func (j ExportJob) MarkProcessing() (ExportJob, error) {
	if j.status != ExportPending {
		return ExportJob{}, ErrExportNotPending
	}
	cp := j
	cp.status = ExportProcessing
	return cp, nil
}

// MarkCompleted gắn file kết quả + thời điểm hoàn tất + hết hạn (signed URL TTL).
func (j ExportJob) MarkCompleted(filePath string, rowCount, sizeBytes int64, now, expiresAt time.Time) ExportJob {
	cp := j
	cp.status = ExportCompleted
	cp.filePath = filePath
	cp.rowCount = rowCount
	cp.fileSizeBytes = sizeBytes
	c := now
	cp.completedAt = &c
	e := expiresAt
	cp.expiresAt = &e
	return cp
}

// MarkFailed chuyển sang failed (kèm thời điểm).
func (j ExportJob) MarkFailed(now time.Time) ExportJob {
	cp := j
	cp.status = ExportFailed
	c := now
	cp.completedAt = &c
	return cp
}

func (t ExportType) valid() bool { return t >= ExportLogs && t <= ExportReport }

func copyParams(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
