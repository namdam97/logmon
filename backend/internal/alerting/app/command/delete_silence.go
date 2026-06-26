package command

import (
	"context"

	"github.com/namdam97/logmon/backend/internal/alerting/ports"
)

// DeleteSilenceHandler huỷ một silence qua gateway (proxy Alertmanager).
type DeleteSilenceHandler struct {
	gateway ports.SilenceGateway
}

// NewDeleteSilenceHandler tạo handler với gateway được inject.
func NewDeleteSilenceHandler(gateway ports.SilenceGateway) *DeleteSilenceHandler {
	return &DeleteSilenceHandler{gateway: gateway}
}

// Handle huỷ silence theo id. Trả ErrSilenceNotFound nếu id không tồn tại.
func (h *DeleteSilenceHandler) Handle(ctx context.Context, id string) error {
	return h.gateway.Delete(ctx, id)
}
