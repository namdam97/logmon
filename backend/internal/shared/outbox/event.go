// Package outbox cài đặt transactional outbox (ADR-016) cho cross-BC domain
// events: ghi event cùng TX với state change, relay nhặt và dispatch qua
// in-process bus. Thuộc shared kernel, dùng chung mọi bounded context.
package outbox

import "time"

// Status là trạng thái xử lý của một outbox event.
type Status string

// Các trạng thái xử lý của outbox event.
const (
	StatusPending   Status = "pending"
	StatusPublished Status = "published"
	StatusFailed    Status = "failed"
)

// Tuning baseline (ADR-016): poll 1s, batch 100, quá 5 lần retry → failed.
const (
	DefaultPollInterval = time.Second
	DefaultBatchSize    = 100
	DefaultMaxRetries   = 5
)

// Event là một domain event đã ghi vào outbox.
type Event struct {
	ID            int64
	AggregateType string
	AggregateID   string // UUID
	EventType     string
	Payload       []byte // JSON
	RetryCount    int
	CreatedAt     time.Time
}
