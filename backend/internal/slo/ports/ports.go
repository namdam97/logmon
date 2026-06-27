// Package ports khai báo interfaces tầng app của slo BC phụ thuộc (DIP).
// Implementation ở adapters. Transaction dùng pattern tx-in-context giống
// alerting BC: SLO INSERT + outbox INSERT nằm chung một TX (transactional outbox).
package ports

import (
	"context"
	"time"

	"github.com/namdam97/logmon/backend/internal/slo/domain"
)

// TxManager chạy fn trong một transaction (tx mang trong ctx ở adapter).
type TxManager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// SLORepository ghi SLO. Save/Update/Delete chạy trong tx của ctx. ExistsByName
// kiểm trùng tên trong workspace (UNIQUE workspace_id+name).
type SLORepository interface {
	Save(ctx context.Context, s domain.SLO) error
	Update(ctx context.Context, s domain.SLO) error
	Delete(ctx context.Context, id domain.SLOID) error
	ExistsByName(ctx context.Context, workspaceID, name string) (bool, error)
}

// SLOReader là read side (CQRS) — truy vấn SLO.
type SLOReader interface {
	ByID(ctx context.Context, id domain.SLOID) (domain.SLO, error)
	List(ctx context.Context, workspaceID string) ([]domain.SLO, error)
	// ListAll trả về mọi SLO (mọi workspace) — dùng để render rule file.
	ListAll(ctx context.Context) ([]domain.SLO, error)
}

// SLOSyncStatusWriter cập nhật trạng thái sync sau khi Syncer render rule file.
// Bulk theo trạng thái pending (giống alerting) — đóng vòng render→reload→ghi DB.
type SLOSyncStatusWriter interface {
	MarkSynced(ctx context.Context, now time.Time) error
	MarkSyncError(ctx context.Context, message string, now time.Time) error
}

// SLOSyncer đồng bộ toàn bộ SLO hiện tại thành rule file Prometheus (recording +
// MWMB alerting), validate, ghi atomic, reload. Idempotent — gọi sau mỗi thay đổi.
type SLOSyncer interface {
	Sync(ctx context.Context) error
}

// SnapshotRepository ghi + đọc budget snapshot (read model cho API/UI).
type SnapshotRepository interface {
	Save(ctx context.Context, snap domain.Snapshot) error
	// Latest trả về snapshot mới nhất của một SLO; ErrSnapshotNotFound nếu chưa có.
	Latest(ctx context.Context, sloID domain.SLOID) (domain.Snapshot, error)
}

// MetricsQuerier thực hiện instant query lên Prometheus/Thanos, trả 1 giá trị
// scalar (dùng cho budget snapshot job). Trả 0 + ErrNoData nếu query rỗng.
type MetricsQuerier interface {
	QueryScalar(ctx context.Context, query string) (float64, error)
}

// EventPublisher ghi domain event vào outbox (trong tx của ctx).
type EventPublisher interface {
	Publish(ctx context.Context, aggregateType, aggregateID, eventType string, payload any) error
}

// IDGenerator sinh định danh SLO (UUID).
type IDGenerator interface {
	NewID() string
}

// Clock cung cấp thời gian hiện tại — inject để test xác định.
type Clock interface {
	Now() time.Time
}
