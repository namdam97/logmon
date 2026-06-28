package health_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/shared/health"
)

func serve(t *testing.T, h gin.HandlerFunc, path string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET(path, h)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func okPing(context.Context) error  { return nil }
func badPing(context.Context) error { return errors.New("down") }

func TestLivenessAlwaysOK(t *testing.T) {
	w := serve(t, health.Liveness(), "/healthz")
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"status":"ok"`)
}

func TestReadinessAllHealthy(t *testing.T) {
	h := health.Readiness(time.Second,
		health.Check{Name: "postgres", Ping: okPing},
		health.Check{Name: "redis", Ping: okPing},
	)
	w := serve(t, h, "/readyz")
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"status":"ready"`)
	require.Contains(t, w.Body.String(), `"postgres":"ok"`)
}

func TestReadinessOneDownReturns503(t *testing.T) {
	h := health.Readiness(time.Second,
		health.Check{Name: "postgres", Ping: okPing},
		health.Check{Name: "elasticsearch", Ping: badPing},
	)
	w := serve(t, h, "/readyz")
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.Contains(t, w.Body.String(), `"status":"not ready"`)
	require.Contains(t, w.Body.String(), `"elasticsearch":"down"`)
	require.Contains(t, w.Body.String(), `"postgres":"ok"`)
}

func TestReadinessNoChecksIsReady(t *testing.T) {
	w := serve(t, health.Readiness(time.Second), "/readyz")
	require.Equal(t, http.StatusOK, w.Code)
}
