package domain

import (
	"strings"
	"time"
)

const minPasswordHashLength = 1

// UserID là value object định danh user (UUID dạng chuỗi, sinh ở tầng adapter).
type UserID struct {
	value string
}

// NewUserID validate và bọc một định danh user không rỗng.
func NewUserID(raw string) (UserID, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return UserID{}, newValidationError("id", "must not be empty")
	}
	return UserID{value: v}, nil
}

// String trả về biểu diễn chuỗi của UserID.
func (id UserID) String() string { return id.value }

// User là aggregate root của bounded context user. Các field không export để
// đảm bảo bất biến — chỉ tạo qua NewUser/Reconstruct.
type User struct {
	id           UserID
	email        Email
	passwordHash string
	createdAt    time.Time
}

// NewUser tạo user mới đã được validate. passwordHash phải là hash đã tính sẵn
// (bcrypt) ở tầng app/adapter — domain không biết thuật toán hashing.
func NewUser(id UserID, email Email, passwordHash string, createdAt time.Time) (User, error) {
	if len(passwordHash) < minPasswordHashLength {
		return User{}, newValidationError("password", "hash must not be empty")
	}
	if createdAt.IsZero() {
		return User{}, newValidationError("createdAt", "must be set")
	}
	return User{id: id, email: email, passwordHash: passwordHash, createdAt: createdAt}, nil
}

// ID trả về định danh user.
func (u User) ID() UserID { return u.id }

// Email trả về email của user.
func (u User) Email() Email { return u.email }

// PasswordHash trả về password hash đã lưu (dùng khi xác thực).
func (u User) PasswordHash() string { return u.passwordHash }

// CreatedAt trả về thời điểm tạo user (UTC).
func (u User) CreatedAt() time.Time { return u.createdAt }
