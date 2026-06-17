// Package promql implement ports.RuleValidator bằng parser chính thức của
// Prometheus (prometheus/promql/parser) — chặn PromQL sai cú pháp trước khi
// rule được lưu/đẩy vào rule sync pipeline (doc_v2/05 §1).
package promql

import (
	"errors"
	"fmt"

	"github.com/prometheus/prometheus/promql/parser"

	"github.com/namdam97/logmon/backend/internal/alerting/ports"
)

// Validator validate biểu thức PromQL.
type Validator struct{}

var _ ports.RuleValidator = (*Validator)(nil)

// NewValidator tạo Validator.
func NewValidator() *Validator { return &Validator{} }

// ValidateExpression parse biểu thức; trả lỗi nếu rỗng hoặc sai cú pháp PromQL.
// Tạo parser mỗi lần gọi (an toàn khi nhiều request đồng thời).
func (Validator) ValidateExpression(expr string) error {
	if expr == "" {
		return errors.New("expression is empty")
	}
	if _, err := parser.NewParser(parser.Options{}).ParseExpr(expr); err != nil {
		return fmt.Errorf("invalid PromQL: %w", err)
	}
	return nil
}
