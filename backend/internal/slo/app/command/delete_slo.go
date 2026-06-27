package command

import (
	"context"
	"fmt"

	"github.com/namdam97/logmon/backend/internal/slo/domain"
	"github.com/namdam97/logmon/backend/internal/slo/ports"
)

// DeleteSLOHandler xoá SLO: kiểm workspace → Delete + phát SLODeleted trong 1 TX
// (syncer sẽ render lại rule file thiếu SLO này).
type DeleteSLOHandler struct {
	tx        ports.TxManager
	repo      ports.SLORepository
	reader    ports.SLOReader
	publisher ports.EventPublisher
}

// NewDeleteSLOHandler tạo handler.
func NewDeleteSLOHandler(tx ports.TxManager, repo ports.SLORepository, reader ports.SLOReader, publisher ports.EventPublisher) *DeleteSLOHandler {
	return &DeleteSLOHandler{tx: tx, repo: repo, reader: reader, publisher: publisher}
}

// Handle xoá SLO theo id trong workspace. Trả ErrSLONotFound nếu không thuộc workspace.
func (h *DeleteSLOHandler) Handle(ctx context.Context, workspaceID, rawID string) error {
	id, err := domain.NewSLOID(rawID)
	if err != nil {
		return err
	}
	existing, err := h.reader.ByID(ctx, id)
	if err != nil {
		return err
	}
	if existing.WorkspaceID() != workspaceID {
		return domain.ErrSLONotFound
	}

	return h.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := h.repo.Delete(ctx, id); err != nil {
			return fmt.Errorf("delete slo: %w", err)
		}
		payload := domain.SLOPayload{SLOID: id.String(), WorkspaceID: workspaceID}
		if err := h.publisher.Publish(ctx, domain.AggregateType, id.String(), domain.EventSLODeleted, payload); err != nil {
			return fmt.Errorf("publish event: %w", err)
		}
		return nil
	})
}
