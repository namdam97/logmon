package command

import (
	"context"
	"fmt"

	"github.com/namdam97/logmon/backend/internal/alerting/domain"
	"github.com/namdam97/logmon/backend/internal/alerting/ports"
)

// DeleteRuleHandler xoá rule và phát event AlertRuleDeleted vào outbox trong
// CÙNG một TX → relay resync để gỡ rule khỏi Prometheus.
type DeleteRuleHandler struct {
	tx        ports.TxManager
	reader    ports.RuleReader
	repo      ports.RuleRepository
	publisher ports.EventPublisher
}

// NewDeleteRuleHandler tạo handler với dependency được inject.
func NewDeleteRuleHandler(
	tx ports.TxManager,
	reader ports.RuleReader,
	repo ports.RuleRepository,
	publisher ports.EventPublisher,
) *DeleteRuleHandler {
	return &DeleteRuleHandler{tx: tx, reader: reader, repo: repo, publisher: publisher}
}

// Handle xoá rule theo id trong workspace. Trả về domain.ErrRuleNotFound nếu
// không tồn tại (hoặc thuộc workspace khác).
func (h *DeleteRuleHandler) Handle(ctx context.Context, workspaceID, rawID string) error {
	id, err := domain.NewRuleID(rawID)
	if err != nil {
		return err
	}
	return h.tx.WithinTx(ctx, func(ctx context.Context) error {
		existing, err := h.reader.ByID(ctx, id)
		if err != nil {
			return err
		}
		if existing.WorkspaceID() != workspaceID {
			return domain.ErrRuleNotFound
		}
		if err := h.repo.Delete(ctx, id); err != nil {
			return fmt.Errorf("delete rule: %w", err)
		}
		payload := domain.RulePayload{RuleID: id.String(), WorkspaceID: workspaceID}
		if err := h.publisher.Publish(ctx, domain.AggregateType, id.String(), domain.EventAlertRuleDeleted, payload); err != nil {
			return fmt.Errorf("publish event: %w", err)
		}
		return nil
	})
}
