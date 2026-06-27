// Package domain chứa aggregate, value object và domain event của bounded
// context slo (Clean Arch + DDD + CQRS). CHỈ import Go standard library.
package domain

import (
	"errors"
	"fmt"
)

// Domain sentinel errors — caller match bằng errors.Is, adapter map sang HTTP.
var (
	// ErrSLONotFound: không tồn tại SLO theo định danh.
	ErrSLONotFound = errors.New("slo not found")
	// ErrSLONameTaken: tên SLO đã tồn tại trong workspace (UNIQUE ws+name).
	ErrSLONameTaken = errors.New("slo name already taken")
	// ErrSnapshotNotFound: chưa có budget snapshot nào cho SLO.
	ErrSnapshotNotFound = errors.New("slo snapshot not found")
	// ErrNoData: query Prometheus trả về rỗng (không có series).
	ErrNoData = errors.New("query returned no data")
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
