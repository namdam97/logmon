package ports

import (
	"context"
	"time"

	"github.com/namdam97/logmon/backend/internal/incident/domain"
)

// Ports cho postmortem & action item (doc_v2/06 §1.5). Postmortem 1-1 với incident.

// PostmortemRepository là write side cho postmortem.
type PostmortemRepository interface {
	Save(ctx context.Context, pm domain.Postmortem) error
	Update(ctx context.Context, pm domain.Postmortem) error
}

// PostmortemReader là read side cho postmortem.
type PostmortemReader interface {
	// ByIncident trả postmortem của incident; ErrPostmortemNotFound nếu chưa có.
	ByIncident(ctx context.Context, incidentID domain.IncidentID) (domain.Postmortem, error)
}

// ActionItemRepository là write side cho action item.
type ActionItemRepository interface {
	Save(ctx context.Context, item domain.ActionItem) error
	Update(ctx context.Context, item domain.ActionItem) error
}

// ActionItemReader là read side cho action item.
type ActionItemReader interface {
	ByID(ctx context.Context, id string) (domain.ActionItem, error)
	ListByPostmortem(ctx context.Context, postmortemID domain.PostmortemID) ([]domain.ActionItem, error)
}

// PostmortemDueReader liệt kê incident SEV1/SEV2 đã Resolved quá hạn mà chưa
// chuyển sang PostmortemPending — đầu vào của reminder sweep (auto 24h).
type PostmortemDueReader interface {
	// ListResolvedNeedingPostmortem trả incident severity SEV1/SEV2, status
	// Resolved, resolvedAt <= before.
	ListResolvedNeedingPostmortem(ctx context.Context, before time.Time) ([]domain.Incident, error)
}
