package query

import (
	"context"

	"github.com/namdam97/logmon/backend/internal/alerting/domain"
	"github.com/namdam97/logmon/backend/internal/alerting/ports"
)

// SilenceQueries là read side (CQRS) cho silence — đọc từ Alertmanager qua gateway.
type SilenceQueries struct {
	gateway ports.SilenceGateway
}

// NewSilenceQueries tạo query với gateway được inject.
func NewSilenceQueries(gateway ports.SilenceGateway) *SilenceQueries {
	return &SilenceQueries{gateway: gateway}
}

// List trả về mọi silence hiện có (kèm trạng thái).
func (q *SilenceQueries) List(ctx context.Context) ([]domain.SilenceView, error) {
	return q.gateway.List(ctx)
}
