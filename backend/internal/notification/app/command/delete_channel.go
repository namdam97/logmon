package command

import (
	"context"
	"fmt"

	"github.com/namdam97/logmon/backend/internal/notification/domain"
	"github.com/namdam97/logmon/backend/internal/notification/ports"
)

// DeleteChannelHandler xóa channel theo workspace + id.
type DeleteChannelHandler struct {
	tx   ports.TxManager
	repo ports.ChannelRepository
}

// NewDeleteChannelHandler tạo handler với dependency được inject.
func NewDeleteChannelHandler(tx ports.TxManager, repo ports.ChannelRepository) *DeleteChannelHandler {
	return &DeleteChannelHandler{tx: tx, repo: repo}
}

// Handle xóa channel. Repo trả ErrChannelNotFound nếu không tồn tại.
func (h *DeleteChannelHandler) Handle(ctx context.Context, workspaceID, rawID string) error {
	id, err := domain.NewChannelID(rawID)
	if err != nil {
		return err
	}
	return h.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := h.repo.Delete(ctx, workspaceID, id); err != nil {
			return fmt.Errorf("delete channel: %w", err)
		}
		return nil
	})
}
