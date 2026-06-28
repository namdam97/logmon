package command_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/reporting/app/command"
	"github.com/namdam97/logmon/backend/internal/reporting/domain"
)

type fakeSchedRepo struct{ saved int }

func (r *fakeSchedRepo) Save(context.Context, domain.ReportSchedule) error { r.saved++; return nil }
func (r *fakeSchedRepo) ByID(context.Context, string, string) (domain.ReportSchedule, error) {
	return domain.ReportSchedule{}, domain.ErrReportScheduleNotFound
}
func (r *fakeSchedRepo) Update(context.Context, domain.ReportSchedule) error { return nil }
func (r *fakeSchedRepo) Delete(context.Context, string, string) error        { return nil }

type fakeJobRepo struct{ saved int }

func (r *fakeJobRepo) Save(context.Context, domain.ExportJob) error { r.saved++; return nil }
func (r *fakeJobRepo) ByID(context.Context, string, string) (domain.ExportJob, error) {
	return domain.ExportJob{}, domain.ErrExportJobNotFound
}
func (r *fakeJobRepo) Update(context.Context, domain.ExportJob) error { return nil }
func (r *fakeJobRepo) ClaimNextPending(context.Context) (domain.ExportJob, bool, error) {
	return domain.ExportJob{}, false, nil
}

type okCron struct{}

func (okCron) Next(_, _ string, after time.Time) (time.Time, error) { return after.Add(time.Hour), nil }

type badCron struct{}

func (badCron) Next(string, string, time.Time) (time.Time, error) {
	return time.Time{}, errors.New("invalid cron")
}

type ids struct{}

func (ids) NewID() string { return "id-1" }

type clk struct{}

func (clk) Now() time.Time { return time.Unix(1000, 0).UTC() }

func TestCreateScheduleValidatesCron(t *testing.T) {
	repo := &fakeSchedRepo{}
	svc := command.NewScheduleService(repo, badCron{}, ids{}, clk{})
	_, err := svc.Create(context.Background(), command.CreateScheduleInput{
		WorkspaceID: "ws-1", ReportType: "slo_weekly", CronExpr: "bogus", Format: "pdf", Recipients: []string{"a@b.c"},
	})
	require.Error(t, err)
	require.Equal(t, 0, repo.saved) // không lưu khi cron sai
}

func TestCreateScheduleOK(t *testing.T) {
	repo := &fakeSchedRepo{}
	svc := command.NewScheduleService(repo, okCron{}, ids{}, clk{})
	s, err := svc.Create(context.Background(), command.CreateScheduleInput{
		WorkspaceID: "ws-1", ReportType: "slo_weekly", CronExpr: "0 9 * * 1", Format: "pdf", Recipients: []string{"a@b.c"},
	})
	require.NoError(t, err)
	require.Equal(t, domain.ReportSLOWeekly, s.ReportType())
	require.Equal(t, 1, repo.saved)
}

func TestCreateExportOK(t *testing.T) {
	repo := &fakeJobRepo{}
	svc := command.NewExportService(repo, ids{}, clk{})
	j, err := svc.Create(context.Background(), command.CreateExportInput{
		WorkspaceID: "ws-1", UserID: "u-1", ExportType: "logs", Format: "csv",
		QueryParams: map[string]any{"service": "api"},
	})
	require.NoError(t, err)
	require.Equal(t, domain.ExportPending, j.Status())
	require.Equal(t, 1, repo.saved)
}

func TestCreateExportRejectsBadType(t *testing.T) {
	svc := command.NewExportService(&fakeJobRepo{}, ids{}, clk{})
	_, err := svc.Create(context.Background(), command.CreateExportInput{
		WorkspaceID: "ws-1", UserID: "u-1", ExportType: "bogus", Format: "csv",
	})
	require.Error(t, err)
}
