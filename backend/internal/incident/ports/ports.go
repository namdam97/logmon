// Package ports khai báo interfaces tầng app của incident BC phụ thuộc (DIP).
// Implementation ở adapters. Transaction dùng pattern tx-in-context: Incident
// UPDATE + timeline INSERT + outbox INSERT nằm chung một TX (transactional outbox).
package ports

import (
	"context"
	"time"

	"github.com/namdam97/logmon/backend/internal/incident/domain"
)

// TxManager chạy fn trong một transaction (tx mang trong ctx ở adapter).
type TxManager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// IncidentRepository là write side — ghi incident. Chạy trong tx của ctx.
type IncidentRepository interface {
	Save(ctx context.Context, inc domain.Incident) error
	Update(ctx context.Context, inc domain.Incident) error
}

// IncidentReader là read side (CQRS) — truy vấn incident.
type IncidentReader interface {
	ByID(ctx context.Context, id domain.IncidentID) (domain.Incident, error)
	List(ctx context.Context, workspaceID string) ([]domain.Incident, error)
	ListActive(ctx context.Context, workspaceID string) ([]domain.Incident, error)
	// ActiveBySourceRef trả về incident đang active cùng source+ref (dedup
	// auto-create); ErrIncidentNotFound nếu không có.
	ActiveBySourceRef(ctx context.Context, workspaceID string, source domain.Source, sourceRef string) (domain.Incident, error)
}

// TimelineRepository ghi + đọc mục timeline của incident.
type TimelineRepository interface {
	Append(ctx context.Context, entry domain.TimelineEntry) error
	List(ctx context.Context, incidentID domain.IncidentID) ([]domain.TimelineEntry, error)
}

// EventPublisher ghi domain event vào outbox (trong tx của ctx).
type EventPublisher interface {
	Publish(ctx context.Context, aggregateType, aggregateID, eventType string, payload any) error
}

// Metrics phát số liệu MTTA/MTTR + counter/gauge incident (doc_v2/06 §1.3).
// Nhãn severity dùng domain.Severity.Label() (cardinality thấp, cố định).
type Metrics interface {
	// Opened: incident vừa tạo — incidents_total++ và open gauge++.
	Opened(severityLabel, service string)
	// Retriaged: severity đổi khi còn active — di chuyển open gauge từ from→to.
	Retriaged(fromLabel, toLabel string)
	// Assigned: ghi nhận MTTA (chỉ lần assign đầu tiên).
	Assigned(mtta time.Duration)
	// Resolved: open gauge-- và ghi nhận MTTR.
	Resolved(severityLabel string, mttr time.Duration)
}

// IDGenerator sinh định danh (UUID) cho incident + timeline entry.
type IDGenerator interface {
	NewID() string
}

// Clock cung cấp thời gian hiện tại — inject để test xác định.
type Clock interface {
	Now() time.Time
}
