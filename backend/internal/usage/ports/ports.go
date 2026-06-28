// Package ports khai báo interface usage BC phụ thuộc (DIP). Implementation ở
// adapters (postgres quota, Prometheus/ES usage readers).
package ports

import (
	"context"
	"time"

	"github.com/namdam97/logmon/backend/internal/usage/domain"
)

// QuotaRepository lưu hạn mức per workspace.
type QuotaRepository interface {
	// Get trả quota; domain.ErrQuotaNotFound nếu chưa cấu hình.
	Get(ctx context.Context, workspaceID string) (domain.Quota, error)
	// Upsert chèn/cập nhật quota (unique theo workspace).
	Upsert(ctx context.Context, q domain.Quota) error
}

// UsageReader đọc usage thực tế: ingestion từ Prometheus (logmon_ingested_bytes),
// storage + log count từ Elasticsearch. Trả 0 + nil khi nguồn không khả dụng
// (degrade an toàn — usage là thông tin, không chặn nghiệp vụ).
type UsageReader interface {
	IngestionBytes(ctx context.Context, workspaceID string, since time.Time) (int64, error)
	StorageBytes(ctx context.Context, workspaceID string) (int64, error)
	LogCount(ctx context.Context, workspaceID string, since time.Time) (int64, error)
}

// Clock cung cấp thời gian hiện tại — inject để test xác định.
type Clock interface {
	Now() time.Time
}
