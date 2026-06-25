// Package domain chứa aggregate, value object và domain event của bounded
// context alerting (Clean Arch + DDD + CQRS). CHỈ import Go standard library.
package domain

import (
	"errors"
	"fmt"
)

// Domain sentinel errors — caller match bằng errors.Is, adapter map sang HTTP.
var (
	// ErrRuleNotFound: không tồn tại alert rule theo định danh.
	ErrRuleNotFound = errors.New("alert rule not found")
	// ErrRuleNameTaken: tên rule đã tồn tại trong workspace (UNIQUE ws+name).
	ErrRuleNameTaken = errors.New("alert rule name already taken")
	// ErrInstanceNotFound: không tồn tại alert instance theo định danh.
	ErrInstanceNotFound = errors.New("alert instance not found")
	// ErrInstanceNotAcknowledgeable: chỉ instance đang firing mới ack được.
	ErrInstanceNotAcknowledgeable = errors.New("alert instance is not firing")
)

// ValidationError mô tả input không hợp lệ trên một field.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation: %s: %s", e.Field, e.Message)
}

func newValidationError(field, message string) *ValidationError {
	return &ValidationError{Field: field, Message: message}
}
