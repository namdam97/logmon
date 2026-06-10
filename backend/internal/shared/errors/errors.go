// Package errors cung cấp error types dùng chung cho toàn bộ bounded contexts:
// sentinel errors cho các trạng thái nghiệp vụ phổ biến và ValidationError cho
// lỗi validate đầu vào tại biên hệ thống.
package errors

import (
	"errors"
	"fmt"
)

// Sentinel errors — caller dùng errors.Is để match, adapter map sang HTTP status.
var (
	// ErrNotFound: không tìm thấy resource theo định danh.
	ErrNotFound = errors.New("resource not found")
	// ErrConflict: vi phạm ràng buộc duy nhất (vd email đã tồn tại).
	ErrConflict = errors.New("resource conflict")
	// ErrUnauthorized: thiếu hoặc sai credentials.
	ErrUnauthorized = errors.New("unauthorized")
)

// ValidationError mô tả một input không hợp lệ trên một field cụ thể.
// Suffix "Error" theo Go style guide.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation: %s: %s", e.Field, e.Message)
}

// NewValidationError tạo ValidationError cho field với message giải thích.
func NewValidationError(field, message string) *ValidationError {
	return &ValidationError{Field: field, Message: message}
}

// AsValidationError trả về *ValidationError nếu err (hoặc wrapped chain) là
// một validation error, cùng ok=true; ngược lại ok=false.
func AsValidationError(err error) (*ValidationError, bool) {
	var ve *ValidationError
	if errors.As(err, &ve) {
		return ve, true
	}
	return nil, false
}
