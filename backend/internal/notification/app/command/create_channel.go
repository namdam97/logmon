// Package command chứa write-side use cases của notification BC: CRUD channel.
package command

import (
	"context"
	"fmt"

	"github.com/namdam97/logmon/backend/internal/notification/domain"
	"github.com/namdam97/logmon/backend/internal/notification/ports"
)

// CreateChannelInput là dữ liệu vào tạo channel.
type CreateChannelInput struct {
	WorkspaceID string
	Name        string
	ChannelType string
	Config      map[string]string
	Events      []string
}

// CreateChannelHandler tạo channel: validate, kiểm trùng tên (UNIQUE workspace+name),
// persist (config mã hóa ở repo) trong một TX.
type CreateChannelHandler struct {
	tx    ports.TxManager
	repo  ports.ChannelRepository
	ids   ports.IDGenerator
	clock ports.Clock
}

// NewCreateChannelHandler tạo handler với dependency được inject.
func NewCreateChannelHandler(tx ports.TxManager, repo ports.ChannelRepository, ids ports.IDGenerator, clock ports.Clock) *CreateChannelHandler {
	return &CreateChannelHandler{tx: tx, repo: repo, ids: ids, clock: clock}
}

// Handle tạo channel mới. Trả ValidationError, ErrChannelNameTaken, hoặc lỗi hạ tầng.
func (h *CreateChannelHandler) Handle(ctx context.Context, in CreateChannelInput) (domain.Channel, error) {
	ct, err := domain.NewChannelType(in.ChannelType)
	if err != nil {
		return domain.Channel{}, err
	}
	id, err := domain.NewChannelID(h.ids.NewID())
	if err != nil {
		return domain.Channel{}, fmt.Errorf("new channel id: %w", err)
	}
	ch, err := domain.NewChannel(domain.NewChannelInput{
		ID:          id,
		WorkspaceID: in.WorkspaceID,
		Name:        in.Name,
		ChannelType: ct,
		Config:      in.Config,
		Events:      in.Events,
		CreatedAt:   h.clock.Now(),
	})
	if err != nil {
		return domain.Channel{}, err
	}

	err = h.tx.WithinTx(ctx, func(ctx context.Context) error {
		exists, err := h.repo.ExistsByName(ctx, ch.WorkspaceID(), ch.Name())
		if err != nil {
			return fmt.Errorf("check name: %w", err)
		}
		if exists {
			return domain.ErrChannelNameTaken
		}
		if err := h.repo.Save(ctx, ch); err != nil {
			return fmt.Errorf("save channel: %w", err)
		}
		return nil
	})
	if err != nil {
		return domain.Channel{}, err
	}
	return ch, nil
}
