// Package domain chứa entity, value object và domain error của bounded context
// user. Theo kiến trúc, package này CHỈ import Go standard library.
package domain

import (
	"errors"
	"fmt"
)

// Domain sentinel errors — caller match bằng errors.Is, adapter map sang HTTP.
var (
	// ErrUserNotFound: không tồn tại user theo định danh.
	ErrUserNotFound = errors.New("user not found")
	// ErrEmailTaken: email đã được đăng ký bởi user khác.
	ErrEmailTaken = errors.New("email already taken")
	// ErrInvalidCredentials: sai email hoặc mật khẩu khi đăng nhập. Dùng chung
	// cho cả hai trường hợp để không lộ email nào tồn tại.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrRefreshTokenInvalid: refresh token không tồn tại, hết hạn, hoặc đã rotate.
	ErrRefreshTokenInvalid = errors.New("invalid refresh token")
	// ErrRefreshTokenReused: refresh token đã rotate bị dùng lại — dấu hiệu token
	// bị đánh cắp; toàn bộ family bị thu hồi.
	ErrRefreshTokenReused = errors.New("refresh token reused")
)

// ValidationError mô tả input không hợp lệ trên một field. Suffix "Error" theo
// Go style guide.
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
