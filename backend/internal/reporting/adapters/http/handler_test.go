package http_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	reporthttp "github.com/namdam97/logmon/backend/internal/reporting/adapters/http"
	"github.com/namdam97/logmon/backend/internal/reporting/app/command"
	"github.com/namdam97/logmon/backend/internal/reporting/domain"
)

type stubSchedCmds struct{ sch domain.ReportSchedule }

func (s stubSchedCmds) Create(context.Context, command.CreateScheduleInput) (domain.ReportSchedule, error) {
	return s.sch, nil
}
func (s stubSchedCmds) SetEnabled(context.Context, string, string, bool) (domain.ReportSchedule, error) {
	return s.sch, nil
}
func (s stubSchedCmds) Delete(context.Context, string, string) error { return nil }

type stubExportCmds struct{ job domain.ExportJob }

func (s stubExportCmds) Create(context.Context, command.CreateExportInput) (domain.ExportJob, error) {
	return s.job, nil
}

type stubQueries struct {
	list []domain.ReportSchedule
	job  domain.ExportJob
	err  error
}

func (s stubQueries) ListSchedules(context.Context, string) ([]domain.ReportSchedule, error) {
	return s.list, s.err
}
func (s stubQueries) GetExportJob(context.Context, string, string) (domain.ExportJob, error) {
	return s.job, s.err
}

type stubSigner struct{}

func (stubSigner) SignedURL(context.Context, string, time.Duration) (string, error) {
	return "https://s3/x", nil
}

func mkSchedule(t *testing.T) domain.ReportSchedule {
	t.Helper()
	s, err := domain.NewReportSchedule(domain.NewScheduleInput{
		ID: "s-1", WorkspaceID: "ws-1", ReportType: domain.ReportSLOWeekly, CronExpr: "0 9 * * 1",
		Format: domain.FormatPDF, Recipients: []string{"a@b.c"}, Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)
	return s
}

func mkJob(t *testing.T) domain.ExportJob {
	t.Helper()
	j, err := domain.NewExportJob(domain.NewJobInput{
		ID: "j-1", WorkspaceID: "ws-1", UserID: "u-1", ExportType: domain.ExportLogs,
		Format: domain.FormatCSV, Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)
	return j
}

func router(role string, sch stubSchedCmds, exp stubExportCmds, q stubQueries) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	mw := func(c *gin.Context) {
		c.Set("auth_user_id", "u-1")
		c.Set("auth_workspace_id", "ws-1")
		c.Set("auth_role", role)
		c.Next()
	}
	reporthttp.NewHandler(sch, exp, q, stubSigner{}).Register(r.Group("/api/v1"), mw)
	return r
}

func do(t *testing.T, r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestListSchedulesViewer(t *testing.T) {
	r := router("viewer", stubSchedCmds{}, stubExportCmds{}, stubQueries{list: []domain.ReportSchedule{mkSchedule(t)}})
	w := do(t, r, http.MethodGet, "/api/v1/reports/schedules", "")
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "slo_weekly")
}

func TestCreateScheduleRequiresAdmin(t *testing.T) {
	r := router("editor", stubSchedCmds{sch: mkSchedule(t)}, stubExportCmds{}, stubQueries{})
	w := do(t, r, http.MethodPost, "/api/v1/reports/schedules",
		`{"reportType":"slo_weekly","cronExpression":"0 9 * * 1","format":"pdf","recipients":["a@b.c"]}`)
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestCreateScheduleAdminOK(t *testing.T) {
	r := router("admin", stubSchedCmds{sch: mkSchedule(t)}, stubExportCmds{}, stubQueries{})
	w := do(t, r, http.MethodPost, "/api/v1/reports/schedules",
		`{"reportType":"slo_weekly","cronExpression":"0 9 * * 1","format":"pdf","recipients":["a@b.c"]}`)
	require.Equal(t, http.StatusCreated, w.Code)
}

func TestExportRequiresEditor(t *testing.T) {
	r := router("viewer", stubSchedCmds{}, stubExportCmds{job: mkJob(t)}, stubQueries{})
	w := do(t, r, http.MethodPost, "/api/v1/export/logs", `{"format":"csv"}`)
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestExportLogsAccepted(t *testing.T) {
	r := router("editor", stubSchedCmds{}, stubExportCmds{job: mkJob(t)}, stubQueries{})
	w := do(t, r, http.MethodPost, "/api/v1/export/logs", `{"format":"csv","queryParams":{"service":"api"}}`)
	require.Equal(t, http.StatusAccepted, w.Code)
	require.Contains(t, w.Body.String(), `"status":"pending"`)
}

func TestGetJobNotFound(t *testing.T) {
	r := router("viewer", stubSchedCmds{}, stubExportCmds{}, stubQueries{err: domain.ErrExportJobNotFound})
	w := do(t, r, http.MethodGet, "/api/v1/export/jobs/x", "")
	require.Equal(t, http.StatusNotFound, w.Code)
}
