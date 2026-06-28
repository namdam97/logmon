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

	usagehttp "github.com/namdam97/logmon/backend/internal/usage/adapters/http"
	"github.com/namdam97/logmon/backend/internal/usage/app"
	"github.com/namdam97/logmon/backend/internal/usage/domain"
)

type stubSvc struct {
	usage domain.UsageSummary
	quota domain.Quota
	err   error
}

func (s stubSvc) GetUsage(context.Context, string) (domain.UsageSummary, error) {
	return s.usage, s.err
}
func (s stubSvc) GetQuota(context.Context, string) (domain.Quota, error) { return s.quota, s.err }
func (s stubSvc) SetQuota(_ context.Context, in app.SetQuotaInput) (domain.Quota, error) {
	if s.err != nil {
		return domain.Quota{}, s.err
	}
	return domain.NewQuota(in.WorkspaceID, in.MaxIngestionBytesPerDay, in.MaxStorageBytes, in.RetentionDays, time.Unix(1, 0).UTC())
}

func router(role string, svc stubSvc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	mw := func(c *gin.Context) {
		c.Set("auth_user_id", "u-1")
		c.Set("auth_workspace_id", "ws-1")
		c.Set("auth_role", role)
		c.Next()
	}
	usagehttp.NewHandler(svc).Register(r.Group("/api/v1"), mw)
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

func TestGetUsage(t *testing.T) {
	now := time.Unix(1_000_000, 0).UTC()
	svc := stubSvc{usage: domain.UsageSummary{IngestionBytes: 1024, StorageBytes: 2048, LogCount: 10, EstimatedCostUSD: 1.5, PeriodStart: now, PeriodEnd: now}}
	w := do(t, router("viewer", svc), http.MethodGet, "/api/v1/usage", "")
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"estimatedCostUsd":1.5`)
}

func TestGetQuota(t *testing.T) {
	svc := stubSvc{quota: domain.DefaultQuota("ws-1", time.Unix(1, 0).UTC())}
	w := do(t, router("viewer", svc), http.MethodGet, "/api/v1/usage/quota", "")
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"retentionDays":30`)
}

func TestSetQuotaRequiresAdmin(t *testing.T) {
	w := do(t, router("editor", stubSvc{}), http.MethodPut, "/api/v1/usage/quota",
		`{"maxIngestionBytesPerDay":1,"maxStorageBytes":1,"retentionDays":1}`)
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestSetQuotaAdminOK(t *testing.T) {
	w := do(t, router("admin", stubSvc{}), http.MethodPut, "/api/v1/usage/quota",
		`{"maxIngestionBytesPerDay":5368709120,"maxStorageBytes":53687091200,"retentionDays":14}`)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"retentionDays":14`)
}

func TestSetQuotaInvalid(t *testing.T) {
	w := do(t, router("admin", stubSvc{}), http.MethodPut, "/api/v1/usage/quota",
		`{"maxIngestionBytesPerDay":0,"maxStorageBytes":1,"retentionDays":1}`)
	require.Equal(t, http.StatusBadRequest, w.Code)
}
