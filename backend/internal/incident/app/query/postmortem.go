package query

import (
	"context"

	"github.com/namdam97/logmon/backend/internal/incident/domain"
	"github.com/namdam97/logmon/backend/internal/incident/ports"
)

// PostmortemQueries là read side cho postmortem + action items (workspace-scoped
// qua incident).
type PostmortemQueries struct {
	incidents ports.IncidentReader
	reader    ports.PostmortemReader
	items     ports.ActionItemReader
}

// NewPostmortemQueries tạo read side.
func NewPostmortemQueries(incidents ports.IncidentReader, reader ports.PostmortemReader, items ports.ActionItemReader) *PostmortemQueries {
	return &PostmortemQueries{incidents: incidents, reader: reader, items: items}
}

// GetByIncident trả postmortem + action items của incident (workspace-scoped).
// ErrIncidentNotFound nếu incident lệch workspace; ErrPostmortemNotFound nếu chưa có.
func (q *PostmortemQueries) GetByIncident(ctx context.Context, workspaceID, rawIncidentID string) (domain.Postmortem, []domain.ActionItem, error) {
	id, err := domain.NewIncidentID(rawIncidentID)
	if err != nil {
		return domain.Postmortem{}, nil, err
	}
	inc, err := q.incidents.ByID(ctx, id)
	if err != nil {
		return domain.Postmortem{}, nil, err
	}
	if inc.WorkspaceID() != workspaceID {
		return domain.Postmortem{}, nil, domain.ErrIncidentNotFound
	}
	pm, err := q.reader.ByIncident(ctx, inc.ID())
	if err != nil {
		return domain.Postmortem{}, nil, err
	}
	items, err := q.items.ListByPostmortem(ctx, pm.ID())
	if err != nil {
		return domain.Postmortem{}, nil, err
	}
	return pm, items, nil
}
