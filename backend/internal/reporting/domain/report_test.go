package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewReportSchedule(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	base := NewScheduleInput{
		ID: "s-1", WorkspaceID: "ws-1", ReportType: ReportSLOWeekly, CronExpr: "0 9 * * 1",
		Format: FormatPDF, Recipients: []string{"a@b.c"}, Now: now,
	}

	t.Run("valid + defaults timezone UTC", func(t *testing.T) {
		s, err := NewReportSchedule(base)
		require.NoError(t, err)
		require.Equal(t, "UTC", s.Timezone())
		require.True(t, s.Enabled())
		require.Nil(t, s.LastRunAt())
	})

	t.Run("rejects empty cron", func(t *testing.T) {
		in := base
		in.CronExpr = "  "
		_, err := NewReportSchedule(in)
		require.Error(t, err)
	})

	t.Run("rejects no recipients", func(t *testing.T) {
		in := base
		in.Recipients = nil
		_, err := NewReportSchedule(in)
		require.Error(t, err)
	})
}

func TestReportScheduleTransitions(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	s, err := NewReportSchedule(NewScheduleInput{
		ID: "s-1", WorkspaceID: "ws-1", ReportType: ReportCostMonthly, CronExpr: "0 0 1 * *",
		Format: FormatCSV, Recipients: []string{"a@b.c"}, Timezone: "Asia/Ho_Chi_Minh", Now: now,
	})
	require.NoError(t, err)

	disabled := s.WithEnabled(false)
	require.False(t, disabled.Enabled())
	require.True(t, s.Enabled()) // bất biến

	ran := s.WithLastRun(now.Add(time.Hour))
	require.NotNil(t, ran.LastRunAt())
	require.Nil(t, s.LastRunAt())
}

func TestParseReportTypeFormat(t *testing.T) {
	for _, rt := range []ReportType{ReportSLOWeekly, ReportIncidentSummary, ReportCostMonthly} {
		got, err := ParseReportType(rt.String())
		require.NoError(t, err)
		require.Equal(t, rt, got)
	}
	_, err := ParseReportType("bogus")
	require.Error(t, err)

	for _, f := range []ReportFormat{FormatPDF, FormatCSV} {
		got, err := ParseReportFormat(f.String())
		require.NoError(t, err)
		require.Equal(t, f, got)
	}
}

func TestExportJobLifecycle(t *testing.T) {
	now := time.Unix(2000, 0).UTC()
	j, err := NewExportJob(NewJobInput{
		ID: "j-1", WorkspaceID: "ws-1", UserID: "u-1", ExportType: ExportLogs,
		QueryParams: map[string]any{"service": "api"}, Format: FormatCSV, Now: now,
	})
	require.NoError(t, err)
	require.Equal(t, ExportPending, j.Status())

	proc, err := j.MarkProcessing()
	require.NoError(t, err)
	require.Equal(t, ExportProcessing, proc.Status())
	require.Equal(t, ExportPending, j.Status()) // bất biến

	// process lại job đã processing → lỗi
	_, err = proc.MarkProcessing()
	require.ErrorIs(t, err, ErrExportNotPending)

	done := proc.MarkCompleted("s3://bucket/key", 100, 4096, now, now.Add(24*time.Hour))
	require.Equal(t, ExportCompleted, done.Status())
	require.Equal(t, int64(100), done.RowCount())
	require.NotNil(t, done.ExpiresAt())

	failed := proc.MarkFailed(now)
	require.Equal(t, ExportFailed, failed.Status())
	require.NotNil(t, failed.CompletedAt())
}
