// Package system cung cấp adapter hạ tầng cho alerting BC: sinh ID (UUID) và
// đồng hồ thực — inject vào app layer để giữ domain/app thuần và test xác định.
package system

import (
	"time"

	"github.com/google/uuid"

	"github.com/namdam97/logmon/backend/internal/alerting/ports"
)

// UUIDGenerator sinh định danh rule dạng UUID v4.
type UUIDGenerator struct{}

var _ ports.IDGenerator = (*UUIDGenerator)(nil)

// NewUUIDGenerator tạo generator UUID v4.
func NewUUIDGenerator() *UUIDGenerator { return &UUIDGenerator{} }

// NewID trả về một UUID v4 mới.
func (UUIDGenerator) NewID() string { return uuid.NewString() }

// Clock trả thời gian thực (UTC).
type Clock struct{}

var _ ports.Clock = (*Clock)(nil)

// NewClock tạo clock thực.
func NewClock() *Clock { return &Clock{} }

// Now trả thời điểm hiện tại theo UTC.
func (Clock) Now() time.Time { return time.Now().UTC() }
