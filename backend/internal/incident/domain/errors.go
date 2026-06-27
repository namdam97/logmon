// Package domain chứa aggregate, value object, timeline và domain event của
// bounded context incident (Clean Arch + DDD + CQRS). CHỈ import Go standard
// library — state machine 7 trạng thái (doc_v2/06 §1.1), severity SEV1-4, MTTA/MTTR.
package domain

import (
	"errors"
	"fmt"
)

// Domain sentinel errors — caller match bằng errors.Is, adapter map sang HTTP.
var (
	// ErrIncidentNotFound: không tồn tại incident theo định danh.
	ErrIncidentNotFound = errors.New("incident not found")
	// ErrInvalidTransition: chuyển trạng thái không hợp lệ theo state machine.
	ErrInvalidTransition = errors.New("invalid status transition")
	// ErrScheduleNotFound: không tồn tại on-call schedule theo định danh.
	ErrScheduleNotFound = errors.New("on-call schedule not found")
	// ErrEscalationPolicyNotFound: không tồn tại escalation policy cho workspace.
	ErrEscalationPolicyNotFound = errors.New("escalation policy not found")
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
