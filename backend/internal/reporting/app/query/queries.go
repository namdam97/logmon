// Package query chứa read-side use case của reporting BC (CQRS).
package query

import (
	"context"

	"github.com/namdam97/logmon/backend/internal/reporting/domain"
	"github.com/namdam97/logmon/backend/internal/reporting/ports"
)

// Queries cung cấp read model cho reporting.
type Queries struct {
	schedules ports.ScheduleReader
	jobs      ports.ExportJobRepository
}

// NewQueries tạo service.
func NewQueries(schedules ports.ScheduleReader, jobs ports.ExportJobRepository) *Queries {
	return &Queries{schedules: schedules, jobs: jobs}
}

// ListSchedules trả schedule của workspace.
func (q *Queries) ListSchedules(ctx context.Context, workspaceID string) ([]domain.ReportSchedule, error) {
	return q.schedules.ListByWorkspace(ctx, workspaceID)
}

// GetExportJob trả export job (poll trạng thái); ErrExportJobNotFound nếu khác workspace.
func (q *Queries) GetExportJob(ctx context.Context, workspaceID, id string) (domain.ExportJob, error) {
	return q.jobs.ByID(ctx, workspaceID, id)
}
