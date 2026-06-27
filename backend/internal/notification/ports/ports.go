// Package ports khai báo interfaces tầng app của notification BC phụ thuộc (DIP).
// Implementation ở adapters. notification là CONSUMER chính của domain events
// (alert_fired, slo_budget_warning...): use case send tra kênh đăng ký → render
// template → enqueue; worker tiêu thụ queue → Sender gửi → ghi history.
package ports

import (
	"context"
	"time"

	"github.com/namdam97/logmon/backend/internal/notification/domain"
)

// TxManager chạy fn trong một transaction (tx mang trong ctx ở adapter).
type TxManager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// ChannelRepository ghi channel. Config (chứa secret) được mã hóa at-rest ở
// adapter (qua shared/crypto.Cipher) — app/domain chỉ thấy plaintext.
type ChannelRepository interface {
	Save(ctx context.Context, c domain.Channel) error
	Update(ctx context.Context, c domain.Channel) error
	Delete(ctx context.Context, workspaceID string, id domain.ChannelID) error
	ExistsByName(ctx context.Context, workspaceID, name string) (bool, error)
}

// ChannelReader là read side (config đã GIẢI MÃ). SubscribedTo trả các kênh đang
// bật của workspace có đăng ký eventType — đầu vào của send use case.
type ChannelReader interface {
	ByID(ctx context.Context, workspaceID string, id domain.ChannelID) (domain.Channel, error)
	List(ctx context.Context, workspaceID string) ([]domain.Channel, error)
	SubscribedTo(ctx context.Context, workspaceID, eventType string) ([]domain.Channel, error)
}

// HistoryWriter ghi một bản ghi kết quả gửi (audit "vì sao không nhận").
type HistoryWriter interface {
	Save(ctx context.Context, h domain.HistoryEntry) error
}

// HistoryReader đọc lịch sử gửi (UI debug).
type HistoryReader interface {
	List(ctx context.Context, workspaceID string, limit int) ([]domain.HistoryEntry, error)
}

// Enqueuer đẩy một message đã render vào delivery queue (Redis Streams).
// delay > 0 cho retry có backoff (gửi lại sau khoảng thời gian).
type Enqueuer interface {
	Enqueue(ctx context.Context, msg domain.Message, delay time.Duration) error
}

// QueueItem là một message lấy từ queue kèm định danh để ACK.
type QueueItem struct {
	ID  string
	Msg domain.Message
}

// QueueConsumer tiêu thụ delivery queue (consumer group). Read block tối đa
// block để chờ item mới; Ack xác nhận đã xử lý xong (at-least-once).
type QueueConsumer interface {
	Read(ctx context.Context, maxItems int, block time.Duration) ([]QueueItem, error)
	Ack(ctx context.Context, ids ...string) error
}

// Sender gửi một message qua một kênh cụ thể (slack/webhook/email...). Trả lỗi
// để worker quyết định retry. Implement ở adapters/sender.
type Sender interface {
	Send(ctx context.Context, msg domain.Message) error
}

// IDGenerator sinh định danh channel (UUID).
type IDGenerator interface {
	NewID() string
}

// Clock cung cấp thời gian hiện tại — inject để test xác định.
type Clock interface {
	Now() time.Time
}
