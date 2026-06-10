package middleware_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/shared/logger"
	"github.com/namdam97/logmon/backend/internal/shared/metrics"
	"github.com/namdam97/logmon/backend/internal/shared/middleware"
)

func newEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	log := logger.New(&bytes.Buffer{}, "info")
	mx := metrics.New()
	r := gin.New()
	r.Use(
		middleware.Recovery(log),
		middleware.TraceID(),
		middleware.SecurityHeaders(),
		middleware.Metrics(mx),
		middleware.Logging(log),
	)
	return r
}

func TestTraceIDAndSecurityHeaders(t *testing.T) {
	r := newEngine()
	r.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotEmpty(t, w.Header().Get("X-Trace-Id"))
	require.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	require.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
}

func TestTraceIDHonoursIncomingHeader(t *testing.T) {
	r := newEngine()
	r.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("X-Trace-Id", "incoming-trace")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, "incoming-trace", w.Header().Get("X-Trace-Id"))
}

func TestRecoveryReturns500OnPanic(t *testing.T) {
	r := newEngine()
	r.GET("/boom", func(_ *gin.Context) { panic("kaboom") })

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
}
