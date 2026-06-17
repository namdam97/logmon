// Package command chứa write-side use cases của alerting BC (CQRS).
package command

import (
	"context"
	"fmt"
	"time"

	"github.com/namdam97/logmon/backend/internal/alerting/domain"
	"github.com/namdam97/logmon/backend/internal/alerting/ports"
)

// CreateRuleInput là dữ liệu vào cho use case tạo alert rule.
type CreateRuleInput struct {
	WorkspaceID string
	Name        string
	Expression  string
	Service     string
	Severity    string
	ForDuration time.Duration
	Labels      map[string]string
	Annotations map[string]string
}

// CreateRuleHandler tạo alert rule: validate PromQL + invariant domain, rồi
// persist rule và phát event AlertRuleCreated vào outbox trong CÙNG một TX.
type CreateRuleHandler struct {
	tx        ports.TxManager
	repo      ports.RuleRepository
	publisher ports.EventPublisher
	validator ports.RuleValidator
	ids       ports.IDGenerator
	clock     ports.Clock
}

// NewCreateRuleHandler tạo handler với dependency được inject.
func NewCreateRuleHandler(
	tx ports.TxManager,
	repo ports.RuleRepository,
	publisher ports.EventPublisher,
	validator ports.RuleValidator,
	ids ports.IDGenerator,
	clock ports.Clock,
) *CreateRuleHandler {
	return &CreateRuleHandler{tx: tx, repo: repo, publisher: publisher, validator: validator, ids: ids, clock: clock}
}

// Handle tạo rule. Trả về domain.ValidationError (severity/PromQL/invariant),
// domain.ErrRuleNameTaken, hoặc lỗi hạ tầng.
func (h *CreateRuleHandler) Handle(ctx context.Context, in CreateRuleInput) (domain.AlertRule, error) {
	severity, err := domain.NewSeverity(in.Severity)
	if err != nil {
		return domain.AlertRule{}, err
	}
	if err := h.validator.ValidateExpression(in.Expression); err != nil {
		return domain.AlertRule{}, &domain.ValidationError{Field: "expression", Message: err.Error()}
	}
	id, err := domain.NewRuleID(h.ids.NewID())
	if err != nil {
		return domain.AlertRule{}, fmt.Errorf("new rule id: %w", err)
	}

	rule, err := domain.NewAlertRule(domain.NewAlertRuleInput{
		ID:          id,
		WorkspaceID: in.WorkspaceID,
		Name:        in.Name,
		Expression:  in.Expression,
		Service:     in.Service,
		ForDuration: in.ForDuration,
		Severity:    severity,
		Labels:      in.Labels,
		Annotations: in.Annotations,
		CreatedAt:   h.clock.Now(),
	})
	if err != nil {
		return domain.AlertRule{}, err
	}

	err = h.tx.WithinTx(ctx, func(ctx context.Context) error {
		exists, err := h.repo.ExistsByName(ctx, rule.WorkspaceID(), rule.Name())
		if err != nil {
			return fmt.Errorf("check name: %w", err)
		}
		if exists {
			return domain.ErrRuleNameTaken
		}
		if err := h.repo.Save(ctx, rule); err != nil {
			return fmt.Errorf("save rule: %w", err)
		}
		payload := domain.RulePayload{RuleID: rule.ID().String(), WorkspaceID: rule.WorkspaceID()}
		if err := h.publisher.Publish(ctx, domain.AggregateType, rule.ID().String(), domain.EventAlertRuleCreated, payload); err != nil {
			return fmt.Errorf("publish event: %w", err)
		}
		return nil
	})
	if err != nil {
		return domain.AlertRule{}, err
	}
	return rule, nil
}
