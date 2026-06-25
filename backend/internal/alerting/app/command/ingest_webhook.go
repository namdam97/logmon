package command

import (
	"context"
	"fmt"
	"time"

	"github.com/namdam97/logmon/backend/internal/alerting/domain"
	"github.com/namdam97/logmon/backend/internal/alerting/ports"
)

// WebhookAlert là một alert đơn lẻ trong payload Alertmanager (đã giải mã ở
// adapter HTTP — command không phụ thuộc định dạng wire).
type WebhookAlert struct {
	Status      string // "firing" | "resolved"
	Fingerprint string
	StartsAt    time.Time
	EndsAt      time.Time
	Labels      map[string]string
}

// IngestWebhookInput gom các alert của một lần gọi webhook cho một workspace.
type IngestWebhookInput struct {
	WorkspaceID string
	Alerts      []WebhookAlert
}

// IngestResult tóm tắt số instance đã xử lý.
type IngestResult struct {
	Firing   int
	Resolved int
}

// IngestWebhookHandler ghi nhận alert từ Alertmanager: firing → upsert instance
// (idempotent theo fingerprint), resolved → đánh dấu resolved. Toàn bộ alert của
// một webhook chạy trong CÙNG một TX (all-or-nothing).
type IngestWebhookHandler struct {
	tx    ports.TxManager
	repo  ports.AlertInstanceRepository
	ids   ports.IDGenerator
	clock ports.Clock
}

// NewIngestWebhookHandler tạo handler với dependency được inject.
func NewIngestWebhookHandler(
	tx ports.TxManager,
	repo ports.AlertInstanceRepository,
	ids ports.IDGenerator,
	clock ports.Clock,
) *IngestWebhookHandler {
	return &IngestWebhookHandler{tx: tx, repo: repo, ids: ids, clock: clock}
}

// Handle xử lý batch alert. Trả về domain.ValidationError nếu payload không hợp
// lệ (fingerprint rỗng, status lạ), hoặc lỗi hạ tầng.
func (h *IngestWebhookHandler) Handle(ctx context.Context, in IngestWebhookInput) (IngestResult, error) {
	var res IngestResult
	err := h.tx.WithinTx(ctx, func(ctx context.Context) error {
		for _, a := range in.Alerts {
			fp, err := domain.NewFingerprint(a.Fingerprint)
			if err != nil {
				return err
			}
			switch a.Status {
			case string(domain.InstanceFiring):
				if err := h.ingestFiring(ctx, in.WorkspaceID, fp, a); err != nil {
					return err
				}
				res.Firing++
			case string(domain.InstanceResolved):
				at := a.EndsAt
				if at.IsZero() {
					at = h.clock.Now()
				}
				if err := h.repo.Resolve(ctx, in.WorkspaceID, fp.String(), at); err != nil {
					return fmt.Errorf("resolve instance: %w", err)
				}
				res.Resolved++
			default:
				return &domain.ValidationError{Field: "status", Message: "must be firing or resolved"}
			}
		}
		return nil
	})
	if err != nil {
		return IngestResult{}, err
	}
	return res, nil
}

func (h *IngestWebhookHandler) ingestFiring(ctx context.Context, workspaceID string, fp domain.Fingerprint, a WebhookAlert) error {
	inst, err := domain.NewFiringInstance(domain.NewFiringInstanceInput{
		ID:          h.ids.NewID(),
		WorkspaceID: workspaceID,
		Fingerprint: fp,
		FiredAt:     a.StartsAt,
		Labels:      a.Labels,
	})
	if err != nil {
		return err
	}
	if err := h.repo.UpsertFiring(ctx, inst); err != nil {
		return fmt.Errorf("upsert instance: %w", err)
	}
	return nil
}
