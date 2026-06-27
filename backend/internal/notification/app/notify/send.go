package notify

import (
	"context"
	"fmt"

	"github.com/namdam97/logmon/backend/internal/notification/domain"
	"github.com/namdam97/logmon/backend/internal/notification/ports"
)

// SendInput mô tả một domain event nghiệp vụ cần phát đi. Data là biến template
// (vd alertName, severity...). DedupKey gom trigger/resolve cùng nguồn (PagerDuty).
type SendInput struct {
	WorkspaceID string
	EventType   string
	EventRef    string
	Data        map[string]string
	DedupKey    string
}

// Logger là cổng log tối thiểu cho send (handle once: log warn khi không có kênh).
type Logger interface {
	Warn(ctx context.Context, msg string)
}

// SendHandler tra các kênh đang bật đăng ký event → render template → enqueue
// từng message. Idempotent ở mức "best effort": gọi lại sẽ enqueue lại (worker
// at-least-once chịu trùng; DedupKey giúp kênh khử trùng).
type SendHandler struct {
	reader   ports.ChannelReader
	queue    ports.Enqueuer
	renderer *renderer
	log      Logger
}

// NewSendHandler tạo handler. Trả lỗi nếu template tĩnh không parse được.
func NewSendHandler(reader ports.ChannelReader, queue ports.Enqueuer, log Logger) (*SendHandler, error) {
	r, err := newRenderer()
	if err != nil {
		return nil, err
	}
	return &SendHandler{reader: reader, queue: queue, renderer: r, log: log}, nil
}

// Handle render + enqueue cho mọi kênh đăng ký. Không có kênh → no-op (log). Lỗi
// enqueue một kênh KHÔNG chặn các kênh khác — gom lỗi đầu tiên trả về cuối.
func (h *SendHandler) Handle(ctx context.Context, in SendInput) error {
	channels, err := h.reader.SubscribedTo(ctx, in.WorkspaceID, in.EventType)
	if err != nil {
		return fmt.Errorf("list subscribed channels: %w", err)
	}
	if len(channels) == 0 {
		if h.log != nil {
			h.log.Warn(ctx, "no channel subscribed to "+in.EventType)
		}
		return nil
	}

	subject, body := h.renderer.render(in.EventType, in.Data)

	var firstErr error
	for _, c := range channels {
		msg := domain.Message{
			ChannelID:   c.ID().String(),
			WorkspaceID: c.WorkspaceID(),
			ChannelType: c.Type().String(),
			Config:      c.Config(),
			EventType:   in.EventType,
			EventRef:    in.EventRef,
			Subject:     subject,
			Body:        body,
			DedupKey:    in.DedupKey,
		}
		if err := h.queue.Enqueue(ctx, msg, 0); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("enqueue channel %s: %w", c.ID(), err)
		}
	}
	return firstErr
}
