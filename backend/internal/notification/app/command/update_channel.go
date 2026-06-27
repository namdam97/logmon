package command

import (
	"context"
	"fmt"

	"github.com/namdam97/logmon/backend/internal/notification/domain"
	"github.com/namdam97/logmon/backend/internal/notification/ports"
)

// UpdateChannelInput là dữ liệu vào sửa channel.
type UpdateChannelInput struct {
	WorkspaceID string
	ID          string
	Name        string
	ChannelType string
	Config      map[string]string
	Events      []string
	Enabled     bool
}

// UpdateChannelHandler sửa channel: load → validate đổi field → kiểm trùng tên
// (nếu đổi tên) → persist, trong một TX.
type UpdateChannelHandler struct {
	tx     ports.TxManager
	repo   ports.ChannelRepository
	reader ports.ChannelReader
	clock  ports.Clock
}

// NewUpdateChannelHandler tạo handler với dependency được inject.
func NewUpdateChannelHandler(tx ports.TxManager, repo ports.ChannelRepository, reader ports.ChannelReader, clock ports.Clock) *UpdateChannelHandler {
	return &UpdateChannelHandler{tx: tx, repo: repo, reader: reader, clock: clock}
}

// Handle sửa channel. Trả ErrChannelNotFound, ValidationError, ErrChannelNameTaken.
func (h *UpdateChannelHandler) Handle(ctx context.Context, in UpdateChannelInput) (domain.Channel, error) {
	id, err := domain.NewChannelID(in.ID)
	if err != nil {
		return domain.Channel{}, err
	}
	ct, err := domain.NewChannelType(in.ChannelType)
	if err != nil {
		return domain.Channel{}, err
	}

	var updated domain.Channel
	err = h.tx.WithinTx(ctx, func(ctx context.Context) error {
		current, err := h.reader.ByID(ctx, in.WorkspaceID, id)
		if err != nil {
			return err
		}
		next, err := current.Update(domain.UpdateInput{
			Name:        in.Name,
			ChannelType: ct,
			Config:      in.Config,
			Events:      in.Events,
			Enabled:     in.Enabled,
		}, h.clock.Now())
		if err != nil {
			return err
		}
		if next.Name() != current.Name() {
			exists, err := h.repo.ExistsByName(ctx, in.WorkspaceID, next.Name())
			if err != nil {
				return fmt.Errorf("check name: %w", err)
			}
			if exists {
				return domain.ErrChannelNameTaken
			}
		}
		if err := h.repo.Update(ctx, next); err != nil {
			return fmt.Errorf("update channel: %w", err)
		}
		updated = next
		return nil
	})
	if err != nil {
		return domain.Channel{}, err
	}
	return updated, nil
}
