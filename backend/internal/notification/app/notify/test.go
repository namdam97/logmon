package notify

import (
	"context"
	"fmt"

	"github.com/namdam97/logmon/backend/internal/notification/domain"
	"github.com/namdam97/logmon/backend/internal/notification/ports"
)

// TestHandler gửi một thông báo thử ĐỒNG BỘ qua đúng kênh (POST :id/test) để
// admin kiểm chứng cấu hình ngay (khác đường async của event thật).
type TestHandler struct {
	reader  ports.ChannelReader
	senders map[string]ports.Sender
}

// NewTestHandler tạo handler test với reader + map sender theo channel type.
func NewTestHandler(reader ports.ChannelReader, senders map[string]ports.Sender) *TestHandler {
	return &TestHandler{reader: reader, senders: senders}
}

// Handle load channel (config đã giải mã), dựng message mẫu, gửi đồng bộ. Trả
// ErrChannelNotFound, hoặc lỗi gửi (để handler báo 502).
func (h *TestHandler) Handle(ctx context.Context, workspaceID, rawID string) error {
	id, err := domain.NewChannelID(rawID)
	if err != nil {
		return err
	}
	ch, err := h.reader.ByID(ctx, workspaceID, id)
	if err != nil {
		return err
	}
	sender, ok := h.senders[ch.Type().String()]
	if !ok {
		return fmt.Errorf("unsupported channel type: %s", ch.Type())
	}
	msg := domain.Message{
		ChannelID:   ch.ID().String(),
		WorkspaceID: ch.WorkspaceID(),
		ChannelType: ch.Type().String(),
		Config:      ch.Config(),
		EventType:   "test",
		EventRef:    ch.ID().String(),
		Subject:     "🔔 LogMon: thông báo thử",
		Body:        "Đây là thông báo thử từ LogMon cho kênh \"" + ch.Name() + "\". Nếu bạn nhận được, cấu hình đã đúng.",
		DedupKey:    "test-" + ch.ID().String(),
	}
	if err := sender.Send(ctx, msg); err != nil {
		return fmt.Errorf("test send: %w", err)
	}
	return nil
}
