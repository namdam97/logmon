// Package health cung cấp handler liveness + readiness theo chuẩn production
// (Kubernetes/LB). Liveness chỉ báo process còn sống (KHÔNG ping dependency — tránh
// bị restart oan khi DB chớp tắt); readiness ping các dependency tới hạn để LB
// ngừng route traffic khi chưa sẵn sàng mà không giết process.
package health

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Pinger kiểm tra một dependency; trả nil nếu khoẻ.
type Pinger func(ctx context.Context) error

// Check là một dependency tới hạn cần kiểm trong readiness.
type Check struct {
	Name string
	Ping Pinger
}

// Liveness trả handler luôn 200 khi process còn phục vụ được request. KHÔNG kiểm
// dependency — chỉ xác nhận event loop/HTTP server còn sống.
func Liveness() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}

// Readiness trả handler ping mọi check với deadline chung timeout. Bất kỳ check
// nào lỗi → 503 + chi tiết per-check; tất cả khoẻ → 200. Mỗi check chạy trong
// cùng context có timeout (fail nhanh, không treo probe).
func Readiness(timeout time.Duration, checks ...Check) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()

		results := make(map[string]string, len(checks))
		ready := true
		for _, ch := range checks {
			if err := ch.Ping(ctx); err != nil {
				results[ch.Name] = "down"
				ready = false
				continue
			}
			results[ch.Name] = "ok"
		}

		status := http.StatusOK
		state := "ready"
		if !ready {
			status = http.StatusServiceUnavailable
			state = "not ready"
		}
		c.JSON(status, gin.H{"status": state, "checks": results})
	}
}
