package command

import (
	"context"
	"fmt"
	"time"

	"github.com/namdam97/logmon/backend/internal/alerting/domain"
	"github.com/namdam97/logmon/backend/internal/alerting/ports"
)

// UpdateRuleInput là dữ liệu vào cho use case cập nhật alert rule.
type UpdateRuleInput struct {
	WorkspaceID string
	ID          string
	Name        string
	Expression  string
	Service     string
	Severity    string
	ForDuration time.Duration
	Labels      map[string]string
	Annotations map[string]string
}

// UpdateRuleHandler cập nhật rule: validate PromQL + invariant, persist và phát
// event AlertRuleUpdated vào outbox trong CÙNG một TX (transactional outbox).
type UpdateRuleHandler struct {
	tx        ports.TxManager
	reader    ports.RuleReader
	repo      ports.RuleRepository
	publisher ports.EventPublisher
	validator ports.RuleValidator
	clock     ports.Clock
}

// NewUpdateRuleHandler tạo handler với dependency được inject.
func NewUpdateRuleHandler(
	tx ports.TxManager,
	reader ports.RuleReader,
	repo ports.RuleRepository,
	publisher ports.EventPublisher,
	validator ports.RuleValidator,
	clock ports.Clock,
) *UpdateRuleHandler {
	return &UpdateRuleHandler{tx: tx, reader: reader, repo: repo, publisher: publisher, validator: validator, clock: clock}
}

// Handle cập nhật rule. Trả về domain.ValidationError, domain.ErrRuleNotFound,
// domain.ErrRuleNameTaken, hoặc lỗi hạ tầng.
func (h *UpdateRuleHandler) Handle(ctx context.Context, in UpdateRuleInput) (domain.AlertRule, error) {
	id, err := domain.NewRuleID(in.ID)
	if err != nil {
		return domain.AlertRule{}, err
	}
	severity, err := domain.NewSeverity(in.Severity)
	if err != nil {
		return domain.AlertRule{}, err
	}
	if err := h.validator.ValidateExpression(in.Expression); err != nil {
		return domain.AlertRule{}, &domain.ValidationError{Field: "expression", Message: err.Error()}
	}

	var updated domain.AlertRule
	err = h.tx.WithinTx(ctx, func(ctx context.Context) error {
		existing, err := h.reader.ByID(ctx, id)
		if err != nil {
			return err
		}
		// Cô lập workspace: rule của workspace khác → coi như không tồn tại (GĐ3 multi-tenant).
		if existing.WorkspaceID() != in.WorkspaceID {
			return domain.ErrRuleNotFound
		}
		if in.Name != existing.Name() {
			exists, err := h.repo.ExistsByName(ctx, in.WorkspaceID, in.Name)
			if err != nil {
				return fmt.Errorf("check name: %w", err)
			}
			if exists {
				return domain.ErrRuleNameTaken
			}
		}

		updated, err = existing.Update(domain.UpdateInput{
			Name:        in.Name,
			Expression:  in.Expression,
			Service:     in.Service,
			ForDuration: in.ForDuration,
			Severity:    severity,
			Labels:      in.Labels,
			Annotations: in.Annotations,
		}, h.clock.Now())
		if err != nil {
			return err
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
