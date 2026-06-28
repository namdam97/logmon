package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	topohttp "github.com/namdam97/logmon/backend/internal/topology/adapters/http"
	"github.com/namdam97/logmon/backend/internal/topology/domain"
)

type stubSvc struct {
	graph domain.Graph
	err   error
}

func (s stubSvc) GetTopology(context.Context, string) (domain.Graph, error) {
	return s.graph, s.err
}

func router(svc stubSvc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	mw := func(c *gin.Context) {
		c.Set("auth_user_id", "u-1")
		c.Set("auth_workspace_id", "ws-1")
		c.Set("auth_role", "viewer")
		c.Next()
	}
	topohttp.NewHandler(svc).Register(r.Group("/api/v1"), mw)
	return r
}

func do(t *testing.T, r http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestGetTopology(t *testing.T) {
	g := domain.BuildGraph([]domain.Edge{
		{Source: "gateway", Target: "orders", CallCount: 100, ErrorCount: 10},
	}, time.Unix(1_000, 0).UTC())

	w := do(t, router(stubSvc{graph: g}), "/api/v1/topology")
	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	require.Contains(t, body, `"source":"gateway"`)
	require.Contains(t, body, `"status":"unhealthy"`) // gateway 10%
	require.Contains(t, body, `"errorRate":0.1`)
}

func TestGetTopologyError(t *testing.T) {
	w := do(t, router(stubSvc{err: context.DeadlineExceeded}), "/api/v1/topology")
	require.Equal(t, http.StatusInternalServerError, w.Code)
}
