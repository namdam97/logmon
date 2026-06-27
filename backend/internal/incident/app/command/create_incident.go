// Package command chứa write-side use cases của incident BC (CQRS).
package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/namdam97/logmon/backend/internal/incident/domain"
	"github.com/namdam97/logmon/backend/internal/incident/ports"
)

// CreateIncidentInput là dữ liệu vào cho use case tạo incident (manual hoặc auto).
type CreateIncidentInput struct {
	WorkspaceID string
	Title       string
	Description string
	Service     string
	Severity    string // optional ("" = chưa phân loại); auto-create budget set SEV2
	Source      string // manual | alert | slo_budget
	SourceRef   string // alert fingerprint / slo id — dedup auto-create
	Actor       string // người/hệ thống tạo (ghi timeline)
}

// CreateIncidentHandler tạo incident ở trạng thái Open: persist + ghi timeline
// "created" + phát IncidentCreated vào outbox trong CÙNG một TX. Với nguồn auto
// (alert/slo_budget) có SourceRef, dedup: nếu đã có incident active cùng ref thì
// trả về incident đó (idempotent — tránh trùng khi event lặp).
type CreateIncidentHandler struct {
	tx        ports.TxManager
	repo      ports.IncidentRepository
	reader    ports.IncidentReader
	timeline  ports.TimelineRepository
	publisher ports.EventPublisher
	metrics   ports.Metrics
	ids       ports.IDGenerator
	clock     ports.Clock
}

// NewCreateIncidentHandler tạo handler với dependency được inject.
func NewCreateIncidentHandler(
	tx ports.TxManager,
	repo ports.IncidentRepository,
	reader ports.IncidentReader,
	timeline ports.TimelineRepository,
	publisher ports.EventPublisher,
	metrics ports.Metrics,
	ids ports.IDGenerator,
	clock ports.Clock,
) *CreateIncidentHandler {
	return &CreateIncidentHandler{
		tx: tx, repo: repo, reader: reader, timeline: timeline,
		publisher: publisher, metrics: metrics, ids: ids, clock: clock,
	}
}

// Handle tạo incident mới. Trả domain.ValidationError hoặc lỗi hạ tầng.
func (h *CreateIncidentHandler) Handle(ctx context.Context, in CreateIncidentInput) (domain.Incident, error) {
	source, err := domain.NewSource(in.Source)
	if err != nil {
		return domain.Incident{}, err
	}
	var severity domain.Severity
	if in.Severity != "" {
		severity, err = domain.NewSeverity(in.Severity)
		if err != nil {
			return domain.Incident{}, err
		}
	}

	// Dedup auto-create: nguồn không phải manual + có ref → tái dùng incident active.
	if source != domain.SourceManual && in.SourceRef != "" {
		existing, err := h.reader.ActiveBySourceRef(ctx, in.WorkspaceID, source, in.SourceRef)
		switch {
		case err == nil:
			return existing, nil
		case !errors.Is(err, domain.ErrIncidentNotFound):
			return domain.Incident{}, fmt.Errorf("dedup lookup: %w", err)
		}
	}

	id, err := domain.NewIncidentID(h.ids.NewID())
	if err != nil {
		return domain.Incident{}, fmt.Errorf("new incident id: %w", err)
	}
	now := h.clock.Now()
	inc, err := domain.NewIncident(domain.NewIncidentInput{
		ID:          id,
		WorkspaceID: in.WorkspaceID,
		Title:       in.Title,
		Description: in.Description,
		Service:     in.Service,
		Severity:    severity,
		Source:      source,
		SourceRef:   in.SourceRef,
		CreatedAt:   now,
	})
	if err != nil {
		return domain.Incident{}, err
	}

	err = h.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := h.repo.Save(ctx, inc); err != nil {
			return fmt.Errorf("save incident: %w", err)
		}
		entry, err := domain.NewTimelineEntry(domain.NewTimelineEntryInput{
			ID:         h.ids.NewID(),
			IncidentID: inc.ID().String(),
			Kind:       domain.KindCreated,
			ToStatus:   inc.Status(),
			Actor:      in.Actor,
			Note:       in.Title,
			At:         now,
		})
		if err != nil {
			return err
		}
		if err := h.timeline.Append(ctx, entry); err != nil {
			return fmt.Errorf("append timeline: %w", err)
		}
		if err := h.publisher.Publish(ctx, domain.AggregateType, inc.ID().String(),
			domain.EventIncidentCreated, domain.NewIncidentPayload(inc)); err != nil {
			return fmt.Errorf("publish event: %w", err)
		}
		return nil
	})
	if err != nil {
		return domain.Incident{}, err
	}
	h.metrics.Opened(inc.Severity().Label(), inc.Service())
	return inc, nil
}
