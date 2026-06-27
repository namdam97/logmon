package command

import (
	"context"
	"fmt"

	"github.com/namdam97/logmon/backend/internal/slo/domain"
	"github.com/namdam97/logmon/backend/internal/slo/ports"
)

// UpdateSLOInput là dữ liệu vào cho use case sửa SLO (full replace).
type UpdateSLOInput struct {
	WorkspaceID        string
	ID                 string
	Name               string
	Service            string
	SLIType            string
	LatencyThresholdMs int
	Target             float64
	WindowDays         int
}

// UpdateSLOHandler sửa SLO: load → áp Update (validate) → persist + phát
// SLOUpdated trong 1 TX. Cô lập workspace (workspace khác → ErrSLONotFound).
type UpdateSLOHandler struct {
	tx        ports.TxManager
	repo      ports.SLORepository
	reader    ports.SLOReader
	publisher ports.EventPublisher
	clock     ports.Clock
}

// NewUpdateSLOHandler tạo handler.
func NewUpdateSLOHandler(tx ports.TxManager, repo ports.SLORepository, reader ports.SLOReader, publisher ports.EventPublisher, clock ports.Clock) *UpdateSLOHandler {
	return &UpdateSLOHandler{tx: tx, repo: repo, reader: reader, publisher: publisher, clock: clock}
}

// Handle sửa SLO. Trả ValidationError, ErrSLONotFound, ErrSLONameTaken, hoặc lỗi hạ tầng.
func (h *UpdateSLOHandler) Handle(ctx context.Context, in UpdateSLOInput) (domain.SLO, error) {
	sliType, err := domain.NewSLIType(in.SLIType)
	if err != nil {
		return domain.SLO{}, err
	}
	id, err := domain.NewSLOID(in.ID)
	if err != nil {
		return domain.SLO{}, err
	}

	existing, err := h.reader.ByID(ctx, id)
	if err != nil {
		return domain.SLO{}, err
	}
	if existing.WorkspaceID() != in.WorkspaceID {
		return domain.SLO{}, domain.ErrSLONotFound
	}

	updated, err := existing.Update(domain.UpdateInput{
		Name:               in.Name,
		Service:            in.Service,
		SLIType:            sliType,
		LatencyThresholdMs: in.LatencyThresholdMs,
		Target:             in.Target,
		WindowDays:         in.WindowDays,
	}, h.clock.Now())
	if err != nil {
		return domain.SLO{}, err
	}

	err = h.tx.WithinTx(ctx, func(ctx context.Context) error {
		if updated.Name() != existing.Name() {
			exists, err := h.repo.ExistsByName(ctx, updated.WorkspaceID(), updated.Name())
			if err != nil {
				return fmt.Errorf("check name: %w", err)
			}
			if exists {
				return domain.ErrSLONameTaken
			}
		}
		if err := h.repo.Update(ctx, updated); err != nil {
			return fmt.Errorf("update slo: %w", err)
		}
		payload := domain.SLOPayload{SLOID: updated.ID().String(), WorkspaceID: updated.WorkspaceID()}
		if err := h.publisher.Publish(ctx, domain.AggregateType, updated.ID().String(), domain.EventSLOUpdated, payload); err != nil {
			return fmt.Errorf("publish event: %w", err)
		}
		return nil
	})
	if err != nil {
		return domain.SLO{}, err
	}
	return updated, nil
}
