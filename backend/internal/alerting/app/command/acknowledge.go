package command

import (
	"context"
	"fmt"

	"github.com/namdam97/logmon/backend/internal/alerting/domain"
	"github.com/namdam97/logmon/backend/internal/alerting/ports"
)

// AcknowledgeInput xác định instance cần ack và người thực hiện.
type AcknowledgeInput struct {
	WorkspaceID string
	InstanceID  string
	AckedBy     string // userID đã xác thực (từ JWT)
}

// AcknowledgeHandler chuyển một alert instance đang firing sang acknowledged:
// load instance (scoped workspace) → domain.Acknowledge (state machine) → persist,
// tất cả trong một TX.
type AcknowledgeHandler struct {
	tx     ports.TxManager
	reader ports.AlertInstanceReader
	repo   ports.AlertInstanceRepository
	clock  ports.Clock
}

// NewAcknowledgeHandler tạo handler với dependency được inject.
func NewAcknowledgeHandler(
	tx ports.TxManager,
	reader ports.AlertInstanceReader,
	repo ports.AlertInstanceRepository,
	clock ports.Clock,
) *AcknowledgeHandler {
	return &AcknowledgeHandler{tx: tx, reader: reader, repo: repo, clock: clock}
}

// Handle ack instance. Trả về ErrInstanceNotFound nếu không có,
// ErrInstanceNotAcknowledgeable nếu instance không còn firing.
func (h *AcknowledgeHandler) Handle(ctx context.Context, in AcknowledgeInput) (domain.AlertInstance, error) {
	var out domain.AlertInstance
	err := h.tx.WithinTx(ctx, func(ctx context.Context) error {
		inst, err := h.reader.ByID(ctx, in.WorkspaceID, in.InstanceID)
		if err != nil {
			return err
		}
		acked, err := inst.Acknowledge(in.AckedBy, h.clock.Now())
		if err != nil {
			return err
		}
		if err := h.repo.Acknowledge(ctx, acked); err != nil {
			return fmt.Errorf("acknowledge instance: %w", err)
		}
		out = acked
		return nil
	})
	if err != nil {
		return domain.AlertInstance{}, err
	}
	return out, nil
}
