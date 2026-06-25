package command

import (
	"context"
	"fmt"

	"github.com/namdam97/logmon/backend/internal/alerting/domain"
	"github.com/namdam97/logmon/backend/internal/alerting/ports"
)

// SetRuleEnabledHandler bật/tắt rule và phát event AlertRuleUpdated vào outbox
// trong CÙNG một TX → relay resync (rule tắt bị loại khỏi rule file).
type SetRuleEnabledHandler struct {
	tx        ports.TxManager
	reader    ports.RuleReader
	repo      ports.RuleRepository
	publisher ports.EventPublisher
	clock     ports.Clock
}

// NewSetRuleEnabledHandler tạo handler với dependency được inject.
func NewSetRuleEnabledHandler(
	tx ports.TxManager,
	reader ports.RuleReader,
	repo ports.RuleRepository,
	publisher ports.EventPublisher,
	clock ports.Clock,
) *SetRuleEnabledHandler {
	return &SetRuleEnabledHandler{tx: tx, reader: reader, repo: repo, publisher: publisher, clock: clock}
}

// Handle đặt trạng thái enabled của rule. Trả về domain.ErrRuleNotFound nếu
// không tồn tại (hoặc thuộc workspace khác).
func (h *SetRuleEnabledHandler) Handle(ctx context.Context, workspaceID, rawID string, enabled bool) (domain.AlertRule, error) {
	id, err := domain.NewRuleID(rawID)
	if err != nil {
		return domain.AlertRule{}, err
	}

	var updated domain.AlertRule
	err = h.tx.WithinTx(ctx, func(ctx context.Context) error {
		existing, err := h.reader.ByID(ctx, id)
		if err != nil {
			return err
		}
		if existing.WorkspaceID() != workspaceID {
			return domain.ErrRuleNotFound
		}
		if enabled {
			updated = existing.Enabled(h.clock.Now())
		} else {
			updated = existing.Disabled(h.clock.Now())
		}
		if err := h.repo.Update(ctx, updated); err != nil {
			return fmt.Errorf("update rule: %w", err)
		}
		payload := domain.RulePayload{RuleID: updated.ID().String(), WorkspaceID: updated.WorkspaceID()}
		if err := h.publisher.Publish(ctx, domain.AggregateType, updated.ID().String(), domain.EventAlertRuleUpdated, payload); err != nil {
			return fmt.Errorf("publish event: %w", err)
		}
		return nil
	})
	if err != nil {
		return domain.AlertRule{}, err
	}
	return updated, nil
}
