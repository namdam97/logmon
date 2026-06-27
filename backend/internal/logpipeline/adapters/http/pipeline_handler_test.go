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

	loghttp "github.com/namdam97/logmon/backend/internal/logpipeline/adapters/http"
	"github.com/namdam97/logmon/backend/internal/logpipeline/app/command"
	"github.com/namdam97/logmon/backend/internal/logpipeline/app/query"
	"github.com/namdam97/logmon/backend/internal/logpipeline/domain"
)

// ---- stubs ----

type stubPipelineCmds struct{ cfg domain.PipelineConfig }

func (s stubPipelineCmds) SwitchMode(context.Context, string, string, string) (domain.PipelineConfig, error) {
	return s.cfg, nil
}
func (s stubPipelineCmds) UpdateILM(context.Context, string, string, command.UpdateILMInput, string) (domain.PipelineConfig, error) {
	return s.cfg, nil
}

type stubDLQCmds struct{ res command.RetryResult }

func (s stubDLQCmds) Retry(context.Context, string, []int64) (command.RetryResult, error) {
	return s.res, nil
}

type stubPipelineQueries struct {
	view    query.StatusView
	cfg     domain.PipelineConfig
	entries []domain.DLQEntry
	counts  map[string]int
}

func (s stubPipelineQueries) Status(context.Context, string, string) (query.StatusView, error) {
	return s.view, nil
}
func (s stubPipelineQueries) GetConfig(context.Context, string) (domain.PipelineConfig, error) {
	return s.cfg, nil
}
func (s stubPipelineQueries) ListDLQ(context.Context, string, string, int) ([]domain.DLQEntry, map[string]int, error) {
	return s.entries, s.counts, nil
}
func (s stubPipelineQueries) DataStreams(context.Context, string) ([]domain.DataStreamStat, error) {
	return nil, nil
}

// router gắn workspace + role vào context (giả lập tenantMW).
func pipeRouter(role string, cmds loghttpCmds, dlq loghttpDLQ, q loghttpQueries) *gin.Engine {
	r := gin.New()
	api := r.Group("/api/v1")
	mw := func(c *gin.Context) {
		c.Set("auth_user_id", "u-1")
		c.Set("auth_workspace_id", "ws-1")
		c.Set("auth_role", role)
		c.Next()
	}
	loghttp.NewPipelineHandler(cmds, dlq, q).Register(api, mw)
	return r
}

// Aliases để tránh export interface nội bộ.
type loghttpCmds = interface {
	SwitchMode(context.Context, string, string, string) (domain.PipelineConfig, error)
	UpdateILM(context.Context, string, string, command.UpdateILMInput, string) (domain.PipelineConfig, error)
}
type loghttpDLQ = interface {
	Retry(context.Context, string, []int64) (command.RetryResult, error)
}
type loghttpQueries = interface {
	Status(context.Context, string, string) (query.StatusView, error)
	GetConfig(context.Context, string) (domain.PipelineConfig, error)
	ListDLQ(context.Context, string, string, int) ([]domain.DLQEntry, map[string]int, error)
	DataStreams(context.Context, string) ([]domain.DataStreamStat, error)
}

func do(t *testing.T, r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func sampleConfig(t *testing.T) domain.PipelineConfig {
	t.Helper()
	return domain.DefaultPipelineConfig("ws-1", time.Unix(1, 0).UTC())
}

// ---- tests ----

func TestStatusViewerOK(t *testing.T) {
	q := stubPipelineQueries{view: query.StatusView{Mode: "A", Health: domain.HealthStatus{Elasticsearch: "up"}, DataStreams: 2}}
	r := pipeRouter("viewer", stubPipelineCmds{}, stubDLQCmds{}, q)
	w := do(t, r, http.MethodGet, "/api/v1/pipeline/status", "")
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"mode":"A"`)
	require.Contains(t, w.Body.String(), `"elasticsearch":"up"`)
}

func TestSwitchModeRequiresAdmin(t *testing.T) {
	r := pipeRouter("editor", stubPipelineCmds{cfg: sampleConfig(t)}, stubDLQCmds{}, stubPipelineQueries{})
	w := do(t, r, http.MethodPost, "/api/v1/pipeline/mode", `{"mode":"B"}`)
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestSwitchModeAdminOK(t *testing.T) {
	r := pipeRouter("admin", stubPipelineCmds{cfg: sampleConfig(t)}, stubDLQCmds{}, stubPipelineQueries{})
	w := do(t, r, http.MethodPost, "/api/v1/pipeline/mode", `{"mode":"B"}`)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestUpdateILMRequiresAdmin(t *testing.T) {
	r := pipeRouter("viewer", stubPipelineCmds{cfg: sampleConfig(t)}, stubDLQCmds{}, stubPipelineQueries{})
	w := do(t, r, http.MethodPut, "/api/v1/pipeline/ilm", `{"hotDays":3,"warmDays":10,"deleteDays":60}`)
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestListDLQViewerOK(t *testing.T) {
	now := time.Unix(1, 0).UTC()
	q := stubPipelineQueries{
		entries: []domain.DLQEntry{domain.ReconstructDLQEntry(1, "ws-1", "raw", "reject", "api", 0, domain.DLQPending, now, nil)},
		counts:  map[string]int{"pending": 1},
	}
	r := pipeRouter("viewer", stubPipelineCmds{}, stubDLQCmds{}, q)
	w := do(t, r, http.MethodGet, "/api/v1/pipeline/dlq", "")
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"pending":1`)
}

func TestRetryDLQAdminOK(t *testing.T) {
	r := pipeRouter("admin", stubPipelineCmds{}, stubDLQCmds{res: command.RetryResult{Retried: []int64{1}, Failed: map[int64]string{}}}, stubPipelineQueries{})
	w := do(t, r, http.MethodPost, "/api/v1/pipeline/dlq/retry", `{"ids":[1]}`)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"retried":[1]`)
}

func TestRetryDLQEmptyIDs(t *testing.T) {
	r := pipeRouter("admin", stubPipelineCmds{}, stubDLQCmds{}, stubPipelineQueries{})
	w := do(t, r, http.MethodPost, "/api/v1/pipeline/dlq/retry", `{"ids":[]}`)
	require.Equal(t, http.StatusBadRequest, w.Code)
}
