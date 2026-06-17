// Package query chứa read-side use cases của alerting BC (CQRS).
package query

import (
	"context"
	"fmt"

	"github.com/namdam97/logmon/backend/internal/alerting/domain"
	"github.com/namdam97/logmon/backend/internal/alerting/ports"
)

// RuleQueries phục vụ truy vấn alert rule (read side).
type RuleQueries struct {
	reader ports.RuleReader
}

// NewRuleQueries tạo query service với read port.
func NewRuleQueries(reader ports.RuleReader) *RuleQueries {
	return &RuleQueries{reader: reader}
}

// Get trả về rule theo id; domain.ErrRuleNotFound nếu không tồn tại.
func (q *RuleQueries) Get(ctx context.Context, rawID string) (domain.AlertRule, error) {
	id, err := domain.NewRuleID(rawID)
	if err != nil {
		return domain.AlertRule{}, err
	}
	rule, err := q.reader.ByID(ctx, id)
	if err != nil {
		return domain.AlertRule{}, fmt.Errorf("get rule: %w", err)
	}
	return rule, nil
}

// List trả về các rule của một workspace.
func (q *RuleQueries) List(ctx context.Context, workspaceID string) ([]domain.AlertRule, error) {
	rules, err := q.reader.List(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	return rules, nil
}
