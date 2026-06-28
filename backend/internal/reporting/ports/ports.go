// Package ports khai báo interface tầng app reporting BC phụ thuộc (DIP).
// Implementation ở adapters (postgres, cron, S3, generators).
package ports

import (
	"context"
	"time"

	"github.com/namdam97/logmon/backend/internal/reporting/domain"
)

// --- Persistence ---

// ScheduleRepository quản lý report schedule (write side).
type ScheduleRepository interface {
	Save(ctx context.Context, s domain.ReportSchedule) error
	ByID(ctx context.Context, workspaceID, id string) (domain.ReportSchedule, error)
	Update(ctx context.Context, s domain.ReportSchedule) error
	Delete(ctx context.Context, workspaceID, id string) error
}

// ScheduleReader đọc report schedule (read side).
type ScheduleReader interface {
	ListByWorkspace(ctx context.Context, workspaceID string) ([]domain.ReportSchedule, error)
	// ListEnabled trả mọi schedule đang bật (mọi workspace) cho scheduler runner.
	ListEnabled(ctx context.Context) ([]domain.ReportSchedule, error)
}

// ExportJobRepository quản lý export job + claim pending cho worker.
type ExportJobRepository interface {
	Save(ctx context.Context, j domain.ExportJob) error
	ByID(ctx context.Context, workspaceID, id string) (domain.ExportJob, error)
	Update(ctx context.Context, j domain.ExportJob) error
	// ClaimNextPending lấy 1 job pending (FOR UPDATE SKIP LOCKED) cho worker.
	// ok=false nếu hàng đợi trống.
	ClaimNextPending(ctx context.Context) (domain.ExportJob, bool, error)
}

// --- Infra (orchestration ngoài) ---

// CronScheduler tính lần chạy kế tiếp của một biểu thức cron theo timezone.
type CronScheduler interface {
	Next(expr, timezone string, after time.Time) (time.Time, error)
}

// ReportGenerator sinh nội dung báo cáo (PDF/CSV) cho một schedule.
type ReportGenerator interface {
	Generate(ctx context.Context, s domain.ReportSchedule) ([]byte, error)
}

// Exporter thực hiện export dữ liệu (logs/metrics) → bytes + số dòng.
type Exporter interface {
	Export(ctx context.Context, j domain.ExportJob) (data []byte, rowCount int64, err error)
}

// BlobStore lưu file (S3) + cấp signed URL có hạn.
type BlobStore interface {
	Put(ctx context.Context, key string, data []byte) error
	SignedURL(ctx context.Context, key string, ttl time.Duration) (string, error)
}

// ReportDelivery gửi báo cáo đã sinh tới người nhận (qua channel/email).
type ReportDelivery interface {
	Deliver(ctx context.Context, s domain.ReportSchedule, data []byte) error
}

// Clock cung cấp thời gian hiện tại — inject để test xác định.
type Clock interface {
	Now() time.Time
}

// IDGenerator sinh định danh duy nhất.
type IDGenerator interface {
	NewID() string
}
