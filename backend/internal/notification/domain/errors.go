// Package domain chứa aggregate, value object và domain event của bounded context
// notification (Clean Architecture — domain đơn giản). CHỈ import Go stdlib.
package domain

import (
	"errors"
	"fmt"
)

// Domain sentinel errors.
var (
	// ErrChannelNotFound: không tồn tại channel theo định danh.
	ErrChannelNotFound = errors.New("notification channel not found")
	// ErrChannelNameTaken: tên channel đã tồn tại trong workspace.
	ErrChannelNameTaken = errors.New("notification channel name already taken")
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
