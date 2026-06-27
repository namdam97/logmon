package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/namdam97/logmon/backend/internal/incident/domain"
	"github.com/namdam97/logmon/backend/internal/incident/ports"
)

// Postmortem write-side use cases (doc_v2/06 §1.5): submit/update nội dung,
// publish, thêm/cập nhật action item. Postmortem 1-1 với incident.

// SubmitPostmortemInput là dữ liệu vào submit/cập nhật postmortem của incident.
type SubmitPostmortemInput struct {
	WorkspaceID     string
	IncidentID      string
	RootCause       string
	Impact          domain.Impact
	TimelineSummary string
	LessonsLearned  string
}

// PostmortemHandler quản lý vòng đời postmortem + action item.
type PostmortemHandler struct {
	incidents ports.IncidentReader
	repo      ports.PostmortemRepository
	reader    ports.PostmortemReader
	items     ports.ActionItemRepository
	itemRead  ports.ActionItemReader
	ids       ports.IDGenerator
	clock     ports.Clock
}

// NewPostmortemHandler tạo handler với dependency được inject.
func NewPostmortemHandler(
	incidents ports.IncidentReader,
	repo ports.PostmortemRepository,
	reader ports.PostmortemReader,
	items ports.ActionItemRepository,
	itemRead ports.ActionItemReader,
	ids ports.IDGenerator,
	clock ports.Clock,
) *PostmortemHandler {
	return &PostmortemHandler{
		incidents: incidents, repo: repo, reader: reader,
		items: items, itemRead: itemRead, ids: ids, clock: clock,
	}
}

// Submit tạo (nếu chưa có) hoặc cập nhật nội dung postmortem của incident.
// DurationSeconds tự suy từ MTTR khi input bỏ trống (=0).
func (h *PostmortemHandler) Submit(ctx context.Context, in SubmitPostmortemInput) (domain.Postmortem, error) {
	inc, err := h.loadIncident(ctx, in.WorkspaceID, in.IncidentID)
	if err != nil {
		return domain.Postmortem{}, err
	}
	impact := in.Impact
	if impact.DurationSeconds == 0 {
		if mttr, ok := inc.MTTR(); ok {
			impact.DurationSeconds = int64(mttr.Seconds())
		}
	}
	now := h.clock.Now()

	existing, err := h.reader.ByIncident(ctx, inc.ID())
	switch {
	case err == nil:
		updated, uerr := existing.UpdateContent(domain.UpdateContentInput{
			RootCause:       in.RootCause,
			Impact:          impact,
			TimelineSummary: in.TimelineSummary,
			LessonsLearned:  in.LessonsLearned,
			Now:             now,
		})
		if uerr != nil {
			return domain.Postmortem{}, uerr
		}
		if err := h.repo.Update(ctx, updated); err != nil {
			return domain.Postmortem{}, fmt.Errorf("update postmortem: %w", err)
		}
		return updated, nil
	case errors.Is(err, domain.ErrPostmortemNotFound):
		pm, perr := domain.NewPostmortem(domain.NewPostmortemInput{
			ID:              h.ids.NewID(),
			IncidentID:      inc.ID(),
			WorkspaceID:     inc.WorkspaceID(),
			RootCause:       in.RootCause,
			Impact:          impact,
			TimelineSummary: in.TimelineSummary,
			LessonsLearned:  in.LessonsLearned,
			Now:             now,
		})
		if perr != nil {
			return domain.Postmortem{}, perr
		}
		if err := h.repo.Save(ctx, pm); err != nil {
			return domain.Postmortem{}, fmt.Errorf("save postmortem: %w", err)
		}
		return pm, nil
	default:
		return domain.Postmortem{}, fmt.Errorf("load postmortem: %w", err)
	}
}

// Publish chốt postmortem của incident (draft→published).
func (h *PostmortemHandler) Publish(ctx context.Context, workspaceID, incidentID string) (domain.Postmortem, error) {
	inc, err := h.loadIncident(ctx, workspaceID, incidentID)
	if err != nil {
		return domain.Postmortem{}, err
	}
	pm, err := h.reader.ByIncident(ctx, inc.ID())
	if err != nil {
		return domain.Postmortem{}, err
	}
	published, err := pm.Publish(h.clock.Now())
	if err != nil {
		return domain.Postmortem{}, err
	}
	if err := h.repo.Update(ctx, published); err != nil {
		return domain.Postmortem{}, fmt.Errorf("update postmortem: %w", err)
	}
	return published, nil
}

// AddActionItemInput là dữ liệu vào thêm action item.
type AddActionItemInput struct {
	WorkspaceID string
	IncidentID  string
	Title       string
	Assignee    string
	DueDate     *time.Time
}

// AddActionItem thêm một action item vào postmortem của incident.
func (h *PostmortemHandler) AddActionItem(ctx context.Context, in AddActionItemInput) (domain.ActionItem, error) {
	inc, err := h.loadIncident(ctx, in.WorkspaceID, in.IncidentID)
	if err != nil {
		return domain.ActionItem{}, err
	}
	pm, err := h.reader.ByIncident(ctx, inc.ID())
	if err != nil {
		return domain.ActionItem{}, err
	}
	item, err := domain.NewActionItem(domain.NewActionItemInput{
		ID:           h.ids.NewID(),
		PostmortemID: pm.ID(),
		Title:        in.Title,
		Assignee:     in.Assignee,
		DueDate:      in.DueDate,
		Now:          h.clock.Now(),
	})
	if err != nil {
		return domain.ActionItem{}, err
	}
	if err := h.items.Save(ctx, item); err != nil {
		return domain.ActionItem{}, fmt.Errorf("save action item: %w", err)
	}
	return item, nil
}

// UpdateActionItemStatus đổi trạng thái một action item (workspace-scoped qua incident).
func (h *PostmortemHandler) UpdateActionItemStatus(ctx context.Context, workspaceID, incidentID, itemID, status string) (domain.ActionItem, error) {
	inc, err := h.loadIncident(ctx, workspaceID, incidentID)
	if err != nil {
		return domain.ActionItem{}, err
	}
	pm, err := h.reader.ByIncident(ctx, inc.ID())
	if err != nil {
		return domain.ActionItem{}, err
	}
	item, err := h.itemRead.ByID(ctx, itemID)
	if err != nil {
		return domain.ActionItem{}, err
	}
	if item.PostmortemID() != pm.ID() {
		return domain.ActionItem{}, domain.ErrActionItemNotFound // không thuộc incident này
	}
	st, err := domain.NewActionItemStatus(status)
	if err != nil {
		return domain.ActionItem{}, err
	}
	updated := item.UpdateStatus(st, h.clock.Now())
	if err := h.items.Update(ctx, updated); err != nil {
		return domain.ActionItem{}, fmt.Errorf("update action item: %w", err)
	}
	return updated, nil
}

// loadIncident đọc incident và xác nhận thuộc workspace (ErrIncidentNotFound nếu lệch).
func (h *PostmortemHandler) loadIncident(ctx context.Context, workspaceID, rawID string) (domain.Incident, error) {
	id, err := domain.NewIncidentID(rawID)
	if err != nil {
		return domain.Incident{}, err
	}
	inc, err := h.incidents.ByID(ctx, id)
	if err != nil {
		return domain.Incident{}, err
	}
	if inc.WorkspaceID() != workspaceID {
		return domain.Incident{}, domain.ErrIncidentNotFound
	}
	return inc, nil
}
