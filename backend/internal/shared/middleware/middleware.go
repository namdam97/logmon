// Package middleware chứa Gin middleware dùng chung: trace_id, request logging,
// panic recovery, metrics và security headers.
package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/namdam97/logmon/backend/internal/shared/logger"
	"github.com/namdam97/logmon/backend/internal/shared/metrics"
)

const traceIDHeader = "X-Trace-Id"

// TraceID gắn một trace_id cho mỗi request (lấy từ header nếu client gửi,
// ngược lại sinh mới bằng crypto/rand) và đưa vào request context.
func TraceID() gin.HandlerFunc {
	return func(c *gin.Context) {
		tid := c.GetHeader(traceIDHeader)
		if tid == "" {
			tid = newTraceID()
		}
		ctx := logger.ContextWithTraceID(c.Request.Context(), tid)
		c.Request = c.Request.WithContext(ctx)
		c.Header(traceIDHeader, tid)
		c.Next()
	}
}

// Logging log mỗi request sau khi xử lý xong, kèm trace_id từ context.
func Logging(log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		ctx := c.Request.Context()
		log.Infof(ctx, "http request", "path", c.FullPath())
	}
}

// Metrics ghi nhận latency và status code của mỗi request theo route template.
func Metrics(m *metrics.Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		path := c.FullPath()
		if path == "" {
			path = "unmatched"
		}
		m.ObserveRequest(c.Request.Method, path, c.Writer.Status(), time.Since(start))
	}
}

// Recovery bắt panic, log lại và trả 500 generic thay vì để service crash.
func Recovery(log *logger.Logger) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, _ any) {
		log.Infof(c.Request.Context(), "panic recovered", "path", c.FullPath())
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "internal server error",
		})
	})
}

// SecurityHeaders set các header bảo mật bắt buộc trên mọi response.
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		c.Next()
	}
}

func newTraceID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand hiếm khi lỗi; fallback timestamp để không bao giờ rỗng.
		return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b)
}
