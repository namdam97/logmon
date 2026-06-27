package query

import (
	"context"
	"time"

	"github.com/namdam97/logmon/backend/internal/incident/domain"
	"github.com/namdam97/logmon/backend/internal/incident/ports"
)

// OnCallQueries là read side cho on-call: liệt kê schedule + tính "ai đang trực"
// tại một thời điểm (pure domain.WhoIsOnCall trên schedule + override hiệu lực).
type OnCallQueries struct {
	schedule  ports.ScheduleReader
	overrides ports.OverrideReader
}

// NewOnCallQueries tạo read side on-call.
func NewOnCallQueries(schedule ports.ScheduleReader, overrides ports.OverrideReader) *OnCallQueries {
	return &OnCallQueries{schedule: schedule, overrides: overrides}
}

// ListSchedules trả về schedule của workspace.
func (q *OnCallQueries) ListSchedules(ctx context.Context, workspaceID string) ([]domain.Schedule, error) {
	return q.schedule.List(ctx, workspaceID)
}

// Current trả về on-call hiện tại của một schedule (workspace-scoped) tại `at`.
// ErrScheduleNotFound nếu schedule không thuộc workspace.
func (q *OnCallQueries) Current(ctx context.Context, workspaceID, rawScheduleID string, at time.Time) (domain.Schedule, domain.OnCall, error) {
	sid, err := domain.NewScheduleID(rawScheduleID)
	if err != nil {
		return domain.Schedule{}, domain.OnCall{}, err
	}
	s, err := q.schedule.ByID(ctx, sid)
	if err != nil {
		return domain.Schedule{}, domain.OnCall{}, err
	}
	if s.WorkspaceID() != workspaceID {
		return domain.Schedule{}, domain.OnCall{}, domain.ErrScheduleNotFound
	}
	active, err := q.overrides.ActiveForSchedule(ctx, sid, at)
	if err != nil {
		return domain.Schedule{}, domain.OnCall{}, err
	}
	return s, domain.WhoIsOnCall(s, active, at), nil
}
