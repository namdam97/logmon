package system

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/reporting/domain"
)

func TestCronNext(t *testing.T) {
	// 09:00 thứ Hai hàng tuần; after = Chủ nhật 2026-01-04 00:00 UTC → next = Thứ 2 05/01 09:00.
	after := time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC)
	next, err := Cron{}.Next("0 9 * * 1", "UTC", after)
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC), next)
}

func TestCronInvalid(t *testing.T) {
	_, err := Cron{}.Next("nonsense", "UTC", time.Unix(0, 0).UTC())
	require.Error(t, err)
	_, err = Cron{}.Next("0 9 * * 1", "Mars/Phobos", time.Unix(0, 0).UTC())
	require.Error(t, err)
}

func TestLocalBlobStore(t *testing.T) {
	dir := t.TempDir()
	bs := NewLocalBlobStore(dir, "https://files.example/exports")
	require.NoError(t, bs.Put(context.Background(), "exports/ws-1/j-1.csv", []byte("a,b")))
	url, err := bs.SignedURL(context.Background(), "exports/ws-1/j-1.csv", time.Hour)
	require.NoError(t, err)
	require.Equal(t, "https://files.example/exports/exports/ws-1/j-1.csv", url)
}

func TestCSVGenerator(t *testing.T) {
	s, err := domain.NewReportSchedule(domain.NewScheduleInput{
		ID: "s-1", WorkspaceID: "ws-1", ReportType: domain.ReportSLOWeekly, CronExpr: "0 9 * * 1",
		Format: domain.FormatCSV, Recipients: []string{"a@b.c"}, Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)
	data, err := CSVGenerator{}.Generate(context.Background(), s)
	require.NoError(t, err)
	require.Contains(t, string(data), "slo_weekly")
}
