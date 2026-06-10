// Package system cung cấp implementation cho các port hạ tầng của user:
// băm mật khẩu (bcrypt), sinh id (UUID v4) và đồng hồ hệ thống.
package system

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/namdam97/logmon/backend/internal/user/ports"
)

// BcryptHasher băm mật khẩu bằng bcrypt với cost cấu hình được.
type BcryptHasher struct {
	cost int
}

var _ ports.PasswordHasher = (*BcryptHasher)(nil)

// NewBcryptHasher tạo hasher; cost <= 0 dùng bcrypt.DefaultCost.
func NewBcryptHasher(cost int) *BcryptHasher {
	if cost <= 0 {
		cost = bcrypt.DefaultCost
	}
	return &BcryptHasher{cost: cost}
}

// Hash trả về bcrypt hash của mật khẩu plaintext.
func (h *BcryptHasher) Hash(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), h.cost)
	if err != nil {
		return "", fmt.Errorf("bcrypt generate: %w", err)
	}
	return string(b), nil
}

// UUIDGenerator sinh id dạng UUID v4.
type UUIDGenerator struct{}

var _ ports.IDGenerator = (*UUIDGenerator)(nil)

// NewUUIDGenerator tạo generator UUID v4.
func NewUUIDGenerator() *UUIDGenerator { return &UUIDGenerator{} }

// NewID trả về một UUID v4 dạng chuỗi.
func (UUIDGenerator) NewID() string { return uuid.NewString() }

// Clock trả về thời gian thực của hệ thống (UTC).
type Clock struct{}

var _ ports.Clock = (*Clock)(nil)

// NewClock tạo đồng hồ hệ thống.
func NewClock() *Clock { return &Clock{} }

// Now trả về thời điểm hiện tại theo UTC.
func (Clock) Now() time.Time { return time.Now().UTC() }
