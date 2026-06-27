// Package query chứa read-side use cases của incident BC (CQRS).
package query

import (
	"context"

	"github.com/namdam97/logmon/backend/internal/incident/domain"
	"github.com/namdam97/logmon/backend/internal/incident/ports"
)

// IncidentQueries là read side cô lập workspace cho incident + timeline.
type IncidentQueries struct {
	reader   ports.IncidentReader
	timeline ports.TimelineRepository
}

// NewIncidentQueries tạo read side.
func NewIncidentQueries(reader ports.IncidentReader, timeline ports.TimelineRepository) *IncidentQueries {
	return &IncidentQueries{reader: reader, timeline: timeline}
}

// Get trả về một incident trong workspace; ErrIncidentNotFound nếu khác workspace.
func (q *IncidentQueries) Get(ctx context.Context, workspaceID, rawID string) (domain.Incident, error) {
	id, err := domain.NewIncidentID(rawID)
	if err != nil {
		return domain.Incident{}, err
	}
	inc, err := q.reader.ByID(ctx, id)
	if err != nil {
		return domain.Incident{}, err
	}
	if inc.WorkspaceID() != workspaceID {
		return domain.Incident{}, domain.ErrIncidentNotFound
	}
	return inc, nil
}

// List trả về mọi incident của workspace (mới nhất trước).
func (q *IncidentQueries) List(ctx context.Context, workspaceID string) ([]domain.Incident, error) {
	return q.reader.List(ctx, workspaceID)
}

// ListActive trả về incident đang active (incident board).
func (q *IncidentQueries) ListActive(ctx context.Context, workspaceID string) ([]domain.Incident, error) {
	return q.reader.ListActive(ctx, workspaceID)
}

// Timeline trả về dòng thời gian của một incident (workspace-scoped).
func (q *IncidentQueries) Timeline(ctx context.Context, workspaceID, rawID string) ([]domain.TimelineEntry, error) {
	inc, err := q.Get(ctx, workspaceID, rawID)
	if err != nil {
		return nil, err
	}
	return q.timeline.List(ctx, inc.ID())
}
