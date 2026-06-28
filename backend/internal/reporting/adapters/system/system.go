// Package system chứa adapter hạ tầng của reporting BC: cron parser (robfig),
// blob store (local FS cho dev — prod thay S3 qua cùng port), và generator/
// exporter/delivery dev (nội dung tối giản — bản prod là nợ kỹ thuật GĐ4).
package system

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/namdam97/logmon/backend/internal/reporting/domain"
	"github.com/namdam97/logmon/backend/internal/reporting/ports"
	apperrors "github.com/namdam97/logmon/backend/internal/shared/errors"
)

// Cron parse biểu thức cron 5-field theo timezone (robfig/cron).
type Cron struct{}

var _ ports.CronScheduler = Cron{}

// Next trả lần chạy kế tiếp sau mốc after theo timezone.
func (Cron) Next(expr, timezone string, after time.Time) (time.Time, error) {
	loc, err := time.LoadLocation(strings.TrimSpace(timezone))
	if err != nil {
		return time.Time{}, apperrors.NewValidationError("timezone", "invalid IANA timezone")
	}
	sched, err := cron.ParseStandard(strings.TrimSpace(expr))
	if err != nil {
		return time.Time{}, apperrors.NewValidationError("cronExpression", "invalid cron expression")
	}
	return sched.Next(after.In(loc)), nil
}

// LocalBlobStore lưu file export xuống thư mục cục bộ (dev). Prod thay bằng S3
// adapter cùng port ports.BlobStore (PutObject + presign).
type LocalBlobStore struct {
	baseDir string
	baseURL string
}

var _ ports.BlobStore = (*LocalBlobStore)(nil)

// NewLocalBlobStore tạo store ghi vào baseDir; SignedURL trả baseURL+key.
func NewLocalBlobStore(baseDir, baseURL string) *LocalBlobStore {
	return &LocalBlobStore{baseDir: baseDir, baseURL: strings.TrimRight(baseURL, "/")}
}

// Put ghi data vào baseDir/key (tạo thư mục cha nếu cần).
func (s *LocalBlobStore) Put(_ context.Context, key string, data []byte) error {
	dst := filepath.Join(s.baseDir, filepath.Clean("/"+key))
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return fmt.Errorf("mkdir export: %w", err)
	}
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		return fmt.Errorf("write export: %w", err)
	}
	return nil
}

// SignedURL trả URL truy cập file (dev: tĩnh, không hết hạn thật).
func (s *LocalBlobStore) SignedURL(_ context.Context, key string, _ time.Duration) (string, error) {
	return s.baseURL + "/" + strings.TrimLeft(key, "/"), nil
}

// CSVGenerator sinh báo cáo tối giản dạng CSV (dev). Bản PDF + dữ liệu thật
// (SLO/incident aggregation) là nợ kỹ thuật GĐ4.
type CSVGenerator struct{}

var _ ports.ReportGenerator = CSVGenerator{}

// Generate trả nội dung CSV header tối giản theo loại báo cáo.
func (CSVGenerator) Generate(_ context.Context, s domain.ReportSchedule) ([]byte, error) {
	content := fmt.Sprintf("report_type,workspace_id,generated\n%s,%s,1\n", s.ReportType().String(), s.WorkspaceID())
	return []byte(content), nil
}

// CSVExporter sinh export tối giản (dev). Truy vấn ES/Prometheus thật là nợ GĐ4.
type CSVExporter struct{}

var _ ports.Exporter = CSVExporter{}

// Export trả CSV header tối giản + rowCount 0.
func (CSVExporter) Export(_ context.Context, j domain.ExportJob) ([]byte, int64, error) {
	content := fmt.Sprintf("export_type,workspace_id\n%s,%s\n", j.ExportType().String(), j.WorkspaceID())
	return []byte(content), 0, nil
}

// LogDelivery "gửi" báo cáo bằng cách no-op (dev). Bản gửi qua channel/email là
// nợ kỹ thuật GĐ4.
type LogDelivery struct{}

var _ ports.ReportDelivery = LogDelivery{}

// Deliver không làm gì (dev).
func (LogDelivery) Deliver(context.Context, domain.ReportSchedule, []byte) error { return nil }
