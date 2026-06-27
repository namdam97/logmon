// Package command chứa write-side use cases của slo BC (CQRS).
package command

import (
	"context"
	"fmt"

	"github.com/namdam97/logmon/backend/internal/slo/domain"
	"github.com/namdam97/logmon/backend/internal/slo/ports"
)

// CreateSLOInput là dữ liệu vào cho use case định nghĩa SLO.
type CreateSLOInput struct {
	WorkspaceID        string
	Name               string
	Service            string
	SLIType            string
	LatencyThresholdMs int
	Target             float64
	WindowDays         int
}

// CreateSLOHandler định nghĩa SLO: validate invariant domain, persist SLO và phát
// event SLODefined vào outbox trong CÙNG một TX → kích hoạt rule sync pipeline.
type CreateSLOHandler struct {
	tx        ports.TxManager
	repo      ports.SLORepository
	publisher ports.EventPublisher
	ids       ports.IDGenerator
	clock     ports.Clock
}

// NewCreateSLOHandler tạo handler với dependency được inject.
func NewCreateSLOHandler(tx ports.TxManager, repo ports.SLORepository, publisher ports.EventPublisher, ids ports.IDGenerator, clock ports.Clock) *CreateSLOHandler {
	return &CreateSLOHandler{tx: tx, repo: repo, publisher: publisher, ids: ids, clock: clock}
}

// Handle định nghĩa SLO mới. Trả domain.ValidationError, domain.ErrSLONameTaken,
// hoặc lỗi hạ tầng.
func (h *CreateSLOHandler) Handle(ctx context.Context, in CreateSLOInput) (domain.SLO, error) {
	sliType, err := domain.NewSLIType(in.SLIType)
	if err != nil {
		return domain.SLO{}, err
	}
	id, err := domain.NewSLOID(h.ids.NewID())
	if err != nil {
		return domain.SLO{}, fmt.Errorf("new slo id: %w", err)
	}

	slo, err := domain.NewSLO(domain.NewSLOInput{
		ID:                 id,
		WorkspaceID:        in.WorkspaceID,
		Name:               in.Name,
		Service:            in.Service,
		SLIType:            sliType,
		LatencyThresholdMs: in.LatencyThresholdMs,
		Target:             in.Target,
		WindowDays:         in.WindowDays,
		CreatedAt:          h.clock.Now(),
	})
	if err != nil {
		return domain.SLO{}, err
	}

	err = h.tx.WithinTx(ctx, func(ctx context.Context) error {
		exists, err := h.repo.ExistsByName(ctx, slo.WorkspaceID(), slo.Name())
		if err != nil {
			return fmt.Errorf("check name: %w", err)
		}
		if exists {
			return domain.ErrSLONameTaken
		}
		if err := h.repo.Save(ctx, slo); err != nil {
			return fmt.Errorf("save slo: %w", err)
		}
		payload := domain.SLOPayload{SLOID: slo.ID().String(), WorkspaceID: slo.WorkspaceID()}
		if err := h.publisher.Publish(ctx, domain.AggregateType, slo.ID().String(), domain.EventSLODefined, payload); err != nil {
			return fmt.Errorf("publish event: %w", err)
		}
		return nil
	})
	if err != nil {
		return domain.SLO{}, err
	}
	return slo, nil
}
