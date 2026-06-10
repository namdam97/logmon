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

func TestTraceIDHonoursValidIncomingHeader(t *testing.T) {
	r := newEngine()
	r.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })

	const valid = "0123456789abcdef0123456789abcdef"
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("X-Trace-Id", valid)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, valid, w.Header().Get("X-Trace-Id"))
}

func TestTraceIDReplacesInvalidIncomingHeader(t *testing.T) {
	r := newEngine()
	r.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("X-Trace-Id", "../../etc/passwd")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	got := w.Header().Get("X-Trace-Id")
	require.NotEqual(t, "../../etc/passwd", got)
	require.Regexp(t, `^[a-f0-9]{32}$`, got)
}

func TestRateLimiterBlocksBurst(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	limiter := middleware.NewPerMinuteLimiter(60, 2) // burst 2
	r.GET("/limited", limiter.Middleware(), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	codes := make([]int, 0, 4)
	for i := 0; i < 4; i++ {
		req := httptest.NewRequest(http.MethodGet, "/limited", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		codes = append(codes, w.Code)
	}
	// 2 request đầu (burst) qua, các request sau bị chặn 429.
	require.Equal(t, http.StatusOK, codes[0])
	require.Equal(t, http.StatusOK, codes[1])
	require.Equal(t, http.StatusTooManyRequests, codes[3])
}

func TestCORSAllowsConfiguredOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.CORS("http://localhost:3000"))
	r.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })

	t.Run("matching origin gets credentials headers", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.Header.Set("Origin", "http://localhost:3000")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, "http://localhost:3000", w.Header().Get("Access-Control-Allow-Origin"))
		require.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
	})

	t.Run("other origin gets no allow-origin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.Header.Set("Origin", "http://evil.com")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("preflight OPTIONS returns 204", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/ping", nil)
		req.Header.Set("Origin", "http://localhost:3000")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusNoContent, w.Code)
	})
}

func TestRecoveryReturns500OnPanic(t *testing.T) {
	r := newEngine()
	r.GET("/boom", func(_ *gin.Context) { panic("kaboom") })

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
}
