// Package command chứa write-side use case của reporting BC: quản lý report
// schedule + tạo export job.
package command

import (
	"context"
	"fmt"

	"github.com/namdam97/logmon/backend/internal/reporting/domain"
	"github.com/namdam97/logmon/backend/internal/reporting/ports"
)

// ScheduleService quản lý vòng đời report schedule.
type ScheduleService struct {
	repo  ports.ScheduleRepository
	cron  ports.CronScheduler
	ids   ports.IDGenerator
	clock ports.Clock
}

// NewScheduleService tạo service với dependencies inject.
func NewScheduleService(repo ports.ScheduleRepository, cron ports.CronScheduler, ids ports.IDGenerator, clock ports.Clock) *ScheduleService {
	return &ScheduleService{repo: repo, cron: cron, ids: ids, clock: clock}
}

// CreateScheduleInput là dữ liệu tạo schedule.
type CreateScheduleInput struct {
	WorkspaceID string
	ReportType  string
	CronExpr    string
	Timezone    string
	Format      string
	Recipients  []string
	ChannelID   string
}

// Create tạo schedule mới (validate cron qua CronScheduler để chặn biểu thức sai).
func (s *ScheduleService) Create(ctx context.Context, in CreateScheduleInput) (domain.ReportSchedule, error) {
	rt, err := domain.ParseReportType(in.ReportType)
	if err != nil {
		return domain.ReportSchedule{}, err
	}
	format, err := domain.ParseReportFormat(in.Format)
	if err != nil {
		return domain.ReportSchedule{}, err
	}
	tz := in.Timezone
	if tz == "" {
		tz = "UTC"
	}
	// Validate cron sớm (fail fast, message rõ) trước khi tạo aggregate.
	if _, err := s.cron.Next(in.CronExpr, tz, s.clock.Now()); err != nil {
		return domain.ReportSchedule{}, err
	}
	sch, err := domain.NewReportSchedule(domain.NewScheduleInput{
		ID: s.ids.NewID(), WorkspaceID: in.WorkspaceID, ReportType: rt, CronExpr: in.CronExpr,
		Timezone: tz, Format: format, Recipients: in.Recipients, ChannelID: in.ChannelID, Now: s.clock.Now(),
	})
	if err != nil {
		return domain.ReportSchedule{}, err
	}
	if err := s.repo.Save(ctx, sch); err != nil {
		return domain.ReportSchedule{}, fmt.Errorf("save schedule: %w", err)
	}
	return sch, nil
}

// SetEnabled bật/tắt schedule.
func (s *ScheduleService) SetEnabled(ctx context.Context, workspaceID, id string, enabled bool) (domain.ReportSchedule, error) {
	cur, err := s.repo.ByID(ctx, workspaceID, id)
	if err != nil {
		return domain.ReportSchedule{}, err
	}
	updated := cur.WithEnabled(enabled)
	if err := s.repo.Update(ctx, updated); err != nil {
		return domain.ReportSchedule{}, fmt.Errorf("update schedule: %w", err)
	}
	return updated, nil
}

// Delete xóa schedule.
func (s *ScheduleService) Delete(ctx context.Context, workspaceID, id string) error {
	if err := s.repo.Delete(ctx, workspaceID, id); err != nil {
		return fmt.Errorf("delete schedule: %w", err)
	}
	return nil
}
