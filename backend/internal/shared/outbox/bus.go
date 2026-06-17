package outbox

import (
	"context"
	"fmt"
	"sync"
)

// Handler xử lý một event. PHẢI idempotent — cùng event có thể được dispatch lại
// (at-least-once) khi relay retry.
type Handler func(ctx context.Context, e Event) error

// Bus là event bus in-process, dispatch đồng bộ tới các handler đã đăng ký theo
// event type. Một process — đủ cho GĐ1-3; khi tách service đổi sang Kafka.
type Bus struct {
	mu       sync.RWMutex
	handlers map[string][]Handler
}

// NewBus tạo một Bus rỗng.
func NewBus() *Bus {
	return &Bus{handlers: make(map[string][]Handler)}
}

// Subscribe đăng ký handler cho một event type (gọi nhiều lần để có nhiều handler).
func (b *Bus) Subscribe(eventType string, h Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], h)
}

// Dispatch gọi tuần tự mọi handler của e.EventType. Trả về lỗi đầu tiên gặp phải
// → relay KHÔNG mark published và sẽ retry. Không có handler nào → nil (no-op).
func (b *Bus) Dispatch(ctx context.Context, e Event) error {
	b.mu.RLock()
	handlers := b.handlers[e.EventType]
	b.mu.RUnlock()

	for _, h := range handlers {
		if err := h(ctx, e); err != nil {
			return fmt.Errorf("dispatch %s: %w", e.EventType, err)
		}
	}
	return nil
}
