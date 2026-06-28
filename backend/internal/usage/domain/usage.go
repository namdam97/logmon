// Package domain chứa entity/value object/domain error của usage BC (GĐ4.5):
// usage per workspace + quota. Chỉ import stdlib + shared/errors.
package domain

import (
	"errors"
	"time"

	apperrors "github.com/namdam97/logmon/backend/internal/shared/errors"
)

// ErrQuotaNotFound báo chưa cấu hình quota cho workspace.
var ErrQuotaNotFound = errors.New("quota not found")

// Đơn giá ước tính chi phí (USD) — cấu hình tĩnh GĐ4; per-workspace pricing là nợ.
const (
	_bytesPerGB          = 1 << 30
	_ingestCostPerGB     = 0.50 // USD / GB log nạp vào
	_storeCostPerGBMonth = 0.10 // USD / GB-tháng lưu trữ
)

// UsageSummary là read model usage của một workspace trong một khoảng thời gian.
type UsageSummary struct {
	WorkspaceID      string
	IngestionBytes   int64
	StorageBytes     int64
	LogCount         int64
	EstimatedCostUSD float64
	PeriodStart      time.Time
	PeriodEnd        time.Time
}

// EstimateCostUSD ước tính chi phí từ lượng nạp + lưu trữ (đơn giá tĩnh).
func EstimateCostUSD(ingestionBytes, storageBytes int64) float64 {
	ingestGB := float64(ingestionBytes) / _bytesPerGB
	storeGB := float64(storageBytes) / _bytesPerGB
	return ingestGB*_ingestCostPerGB + storeGB*_storeCostPerGBMonth
}

// Quota là hạn mức tài nguyên của một workspace. Bất biến — đổi trả bản sao mới.
type Quota struct {
	workspaceID             string
	maxIngestionBytesPerDay int64
	maxStorageBytes         int64
	retentionDays           int
	updatedAt               time.Time
}

// Mặc định hạn mức cho workspace mới (doc_v2/03: retention 30d).
const (
	_defaultMaxIngestPerDay = 10 * _bytesPerGB  // 10 GB/ngày
	_defaultMaxStorage      = 100 * _bytesPerGB // 100 GB
	_defaultRetentionDays   = 30
)

// DefaultQuota tạo quota mặc định cho workspace.
func DefaultQuota(workspaceID string, now time.Time) Quota {
	return Quota{
		workspaceID:             workspaceID,
		maxIngestionBytesPerDay: _defaultMaxIngestPerDay,
		maxStorageBytes:         _defaultMaxStorage,
		retentionDays:           _defaultRetentionDays,
		updatedAt:               now,
	}
}

// NewQuota tạo quota đã validate (mọi hạn mức > 0).
func NewQuota(workspaceID string, maxIngestPerDay, maxStorage int64, retentionDays int, now time.Time) (Quota, error) {
	if maxIngestPerDay <= 0 {
		return Quota{}, apperrors.NewValidationError("maxIngestionBytesPerDay", "must be positive")
	}
	if maxStorage <= 0 {
		return Quota{}, apperrors.NewValidationError("maxStorageBytes", "must be positive")
	}
	if retentionDays <= 0 {
		return Quota{}, apperrors.NewValidationError("retentionDays", "must be positive")
	}
	if now.IsZero() {
		return Quota{}, apperrors.NewValidationError("updatedAt", "must be set")
	}
	return Quota{
		workspaceID: workspaceID, maxIngestionBytesPerDay: maxIngestPerDay,
		maxStorageBytes: maxStorage, retentionDays: retentionDays, updatedAt: now,
	}, nil
}

// ReconstructQuota dựng lại từ storage — KHÔNG validate lại.
func ReconstructQuota(workspaceID string, maxIngestPerDay, maxStorage int64, retentionDays int, updatedAt time.Time) Quota {
	return Quota{
		workspaceID: workspaceID, maxIngestionBytesPerDay: maxIngestPerDay,
		maxStorageBytes: maxStorage, retentionDays: retentionDays, updatedAt: updatedAt,
	}
}

// WorkspaceID trả về workspace.
func (q Quota) WorkspaceID() string { return q.workspaceID }

// MaxIngestionBytesPerDay trả về hạn mức nạp/ngày.
func (q Quota) MaxIngestionBytesPerDay() int64 { return q.maxIngestionBytesPerDay }

// MaxStorageBytes trả về hạn mức lưu trữ.
func (q Quota) MaxStorageBytes() int64 { return q.maxStorageBytes }

// RetentionDays trả về số ngày giữ dữ liệu.
func (q Quota) RetentionDays() int { return q.retentionDays }

// UpdatedAt trả về thời điểm cập nhật gần nhất (UTC).
func (q Quota) UpdatedAt() time.Time { return q.updatedAt }

// IngestionExceeded báo lượng nạp trong ngày đã vượt hạn mức chưa.
func (q Quota) IngestionExceeded(ingestionBytesToday int64) bool {
	return ingestionBytesToday > q.maxIngestionBytesPerDay
}

// StorageExceeded báo lượng lưu trữ đã vượt hạn mức chưa.
func (q Quota) StorageExceeded(storageBytes int64) bool {
	return storageBytes > q.maxStorageBytes
}
