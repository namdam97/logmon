package command

import (
	"context"
	"fmt"
	"time"

	"github.com/namdam97/logmon/backend/internal/incident/domain"
	"github.com/namdam97/logmon/backend/internal/incident/ports"
)

// TransitionHandler thực thi mọi chuyển trạng thái incident: load (trong tx) →
// transition domain (guard state machine) → Update + ghi timeline + (tuỳ) phát
// event, tất cả trong CÙNG một TX. Cập nhật metrics sau commit.
type TransitionHandler struct {
	tx        ports.TxManager
	repo      ports.IncidentRepository
	reader    ports.IncidentReader
	timeline  ports.TimelineRepository
	publisher ports.EventPublisher
	metrics   ports.Metrics
	ids       ports.IDGenerator
	clock     ports.Clock
}

// NewTransitionHandler tạo handler với dependency được inject.
func NewTransitionHandler(
	tx ports.TxManager,
	repo ports.IncidentRepository,
	reader ports.IncidentReader,
	timeline ports.TimelineRepository,
	publisher ports.EventPublisher,
	metrics ports.Metrics,
	ids ports.IDGenerator,
	clock ports.Clock,
) *TransitionHandler {
	return &TransitionHandler{
		tx: tx, repo: repo, reader: reader, timeline: timeline,
		publisher: publisher, metrics: metrics, ids: ids, clock: clock,
	}
}

// transitionSpec mô tả một chuyển trạng thái cụ thể.
type transitionSpec struct {
	workspaceID string
	rawID       string
	actor       string
	note        string
	kind        domain.TimelineKind
	eventType   string // "" = không phát event
	fn          func(domain.Incident, time.Time) (domain.Incident, error)
}

// apply chạy khung chung cho mọi transition. Trả before/after để caller cập nhật
// metrics tùy ngữ cảnh.
func (h *TransitionHandler) apply(ctx context.Context, spec transitionSpec) (before, after domain.Incident, err error) {
	id, err := domain.NewIncidentID(spec.rawID)
	if err != nil {
		return domain.Incident{}, domain.Incident{}, err
	}
	now := h.clock.Now()

	err = h.tx.WithinTx(ctx, func(ctx context.Context) error {
		cur, err := h.reader.ByID(ctx, id)
		if err != nil {
			return err
		}
		if cur.WorkspaceID() != spec.workspaceID {
			return domain.ErrIncidentNotFound
		}
		next, err := spec.fn(cur, now)
		if err != nil {
			return err
		}
		if err := h.repo.Update(ctx, next); err != nil {
			return fmt.Errorf("update incident: %w", err)
		}
		entry, err := domain.NewTimelineEntry(domain.NewTimelineEntryInput{
			ID:         h.ids.NewID(),
			IncidentID: next.ID().String(),
			Kind:       spec.kind,
			FromStatus: cur.Status(),
			ToStatus:   next.Status(),
			Actor:      spec.actor,
			Note:       spec.note,
			At:         now,
		})
		if err != nil {
			return err
		}
		if err := h.timeline.Append(ctx, entry); err != nil {
			return fmt.Errorf("append timeline: %w", err)
		}
		if spec.eventType != "" {
			if err := h.publisher.Publish(ctx, domain.AggregateType, next.ID().String(),
				spec.eventType, domain.NewIncidentPayload(next)); err != nil {
				return fmt.Errorf("publish event: %w", err)
			}
		}
		before, after = cur, next
		return nil
	})
	if err != nil {
		return domain.Incident{}, domain.Incident{}, err
	}
	return before, after, nil
}

// Triage chuyển Open → Triaged: gán severity + impact.
func (h *TransitionHandler) Triage(ctx context.Context, workspaceID, rawID, severityRaw, description, actor string) (domain.Incident, error) {
	severity, err := domain.NewSeverity(severityRaw)
	if err != nil {
		return domain.Incident{}, err
	}
	before, after, err := h.apply(ctx, transitionSpec{
		workspaceID: workspaceID, rawID: rawID, actor: actor, note: description,
		kind: domain.KindTriaged, eventType: domain.EventIncidentTriaged,
		fn: func(i domain.Incident, now time.Time) (domain.Incident, error) {
			return i.Triage(severity, description, now)
		},
	})
	if err != nil {
		return domain.Incident{}, err
	}
	if before.Severity().Label() != after.Severity().Label() {
		h.metrics.Retriaged(before.Severity().Label(), after.Severity().Label())
	}
	return after, nil
}

// Assign chuyển Triaged/Mitigating → Assigned. Ghi MTTA lần assign đầu tiên.
func (h *TransitionHandler) Assign(ctx context.Context, workspaceID, rawID, assignee, actor string) (domain.Incident, error) {
	before, after, err := h.apply(ctx, transitionSpec{
		workspaceID: workspaceID, rawID: rawID, actor: actor, note: "assignee: " + assignee,
		kind: domain.KindAssigned, eventType: domain.EventIncidentAssigned,
		fn: func(i domain.Incident, now time.Time) (domain.Incident, error) {
			return i.Assign(assignee, now)
		},
	})
	if err != nil {
		return domain.Incident{}, err
	}
	if before.AssignedAt() == nil {
		if mtta, ok := after.MTTA(); ok {
			h.metrics.Assigned(mtta)
		}
	}
	return after, nil
}

// StartMitigation chuyển Assigned → Mitigating. Không phát event ra ngoài.
func (h *TransitionHandler) StartMitigation(ctx context.Context, workspaceID, rawID, actor string) (domain.Incident, error) {
	_, after, err := h.apply(ctx, transitionSpec{
		workspaceID: workspaceID, rawID: rawID, actor: actor,
		kind: domain.KindMitigating,
		fn: func(i domain.Incident, now time.Time) (domain.Incident, error) {
			return i.StartMitigation(now)
		},
	})
	if err != nil {
		return domain.Incident{}, err
	}
	return after, nil
}

// Resolve chuyển Open/Mitigating → Resolved. Ghi MTTR + giảm open gauge.
func (h *TransitionHandler) Resolve(ctx context.Context, workspaceID, rawID, note, actor string) (domain.Incident, error) {
	_, after, err := h.apply(ctx, transitionSpec{
		workspaceID: workspaceID, rawID: rawID, actor: actor, note: note,
		kind: domain.KindResolved, eventType: domain.EventIncidentResolved,
		fn: func(i domain.Incident, now time.Time) (domain.Incident, error) {
			return i.Resolve(now)
		},
	})
	if err != nil {
		return domain.Incident{}, err
	}
	mttr, _ := after.MTTR()
	h.metrics.Resolved(after.Severity().Label(), mttr)
	return after, nil
}

// RequirePostmortem chuyển Resolved → PostmortemPending (SEV1/SEV2). Nội bộ.
func (h *TransitionHandler) RequirePostmortem(ctx context.Context, workspaceID, rawID, actor string) (domain.Incident, error) {
	_, after, err := h.apply(ctx, transitionSpec{
		workspaceID: workspaceID, rawID: rawID, actor: actor,
		kind: domain.KindPostmortemPending,
		fn: func(i domain.Incident, now time.Time) (domain.Incident, error) {
			return i.RequirePostmortem(now)
		},
	})
	if err != nil {
		return domain.Incident{}, err
	}
	return after, nil
}

// Close chuyển Resolved (SEV3/4) / PostmortemPending → Closed.
func (h *TransitionHandler) Close(ctx context.Context, workspaceID, rawID, note, actor string) (domain.Incident, error) {
	_, after, err := h.apply(ctx, transitionSpec{
		workspaceID: workspaceID, rawID: rawID, actor: actor, note: note,
		kind: domain.KindClosed, eventType: domain.EventIncidentClosed,
		fn: func(i domain.Incident, now time.Time) (domain.Incident, error) {
			return i.Close(now)
		},
	})
	if err != nil {
		return domain.Incident{}, err
	}
	return after, nil
}
