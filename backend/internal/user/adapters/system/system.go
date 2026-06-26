// Package system cung cấp implementation cho các port hạ tầng của user:
// sinh id (UUID v4) và đồng hồ hệ thống. Băm mật khẩu nằm ở argon2.go.
package system

import (
	"time"

	"github.com/google/uuid"

	"github.com/namdam97/logmon/backend/internal/user/ports"
)

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
