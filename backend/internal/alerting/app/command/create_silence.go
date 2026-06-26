package command

import (
	"context"
	"fmt"
	"time"

	"github.com/namdam97/logmon/backend/internal/alerting/domain"
	"github.com/namdam97/logmon/backend/internal/alerting/ports"
)

// CreateSilenceInput gom dữ liệu tạo silence từ HTTP. StartsAt rỗng → mặc định
// thời điểm hiện tại (silence bắt đầu ngay). CreatedBy là userID đã xác thực.
type CreateSilenceInput struct {
	Matchers  []domain.MatcherInput
	StartsAt  time.Time
	EndsAt    time.Time
	CreatedBy string
	Comment   string
}

// CreateSilenceHandler validate input thành domain.Silence rồi proxy sang
// Alertmanager. Không chạm DB — Alertmanager là source of truth.
type CreateSilenceHandler struct {
	gateway ports.SilenceGateway
	clock   ports.Clock
}

// NewCreateSilenceHandler tạo handler với gateway + clock được inject.
func NewCreateSilenceHandler(gateway ports.SilenceGateway, clock ports.Clock) *CreateSilenceHandler {
	return &CreateSilenceHandler{gateway: gateway, clock: clock}
}

// Handle tạo silence, trả về silenceID. ValidationError nếu input không hợp lệ.
func (h *CreateSilenceHandler) Handle(ctx context.Context, in CreateSilenceInput) (string, error) {
	starts := in.StartsAt
	if starts.IsZero() {
		starts = h.clock.Now()
	}
	s, err := domain.NewSilence(domain.NewSilenceInput{
		Matchers:  in.Matchers,
		StartsAt:  starts,
		EndsAt:    in.EndsAt,
		CreatedBy: in.CreatedBy,
		Comment:   in.Comment,
	})
	if err != nil {
		return "", err
	}
	id, err := h.gateway.Create(ctx, s)
	if err != nil {
		return "", fmt.Errorf("create silence: %w", err)
	}
	return id, nil
}
