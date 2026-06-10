package middleware

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// RateLimiter giới hạn request theo IP bằng token bucket (golang.org/x/time/rate).
// Phù hợp single-instance; multi-instance prod nên dùng store tập trung (Redis).
//
// LƯU Ý: map clients tăng theo số IP thấy được và chưa có cơ chế evict — chấp
// nhận được cho skeleton, cần bổ sung TTL/LRU khi lên production.
type RateLimiter struct {
	mu      sync.Mutex
	clients map[string]*rate.Limiter
	rate    rate.Limit
	burst   int
}

// NewRateLimiter tạo limiter cho phép r request/giây với burst tối đa.
func NewRateLimiter(r rate.Limit, burst int) *RateLimiter {
	return &RateLimiter{
		clients: make(map[string]*rate.Limiter),
		rate:    r,
		burst:   burst,
	}
}

// NewPerMinuteLimiter tạo limiter giới hạn perMinute request/phút mỗi IP, cho
// phép bùng phát tới burst request. Tiện dụng để caller không cần import rate.
func NewPerMinuteLimiter(perMinute, burst int) *RateLimiter {
	return NewRateLimiter(rate.Limit(float64(perMinute)/60.0), burst)
}

func (l *RateLimiter) limiterFor(ip string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	lim, ok := l.clients[ip]
	if !ok {
		lim = rate.NewLimiter(l.rate, l.burst)
		l.clients[ip] = lim
	}
	return lim
}

// Middleware trả về handler chặn request vượt ngưỡng với 429.
func (l *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !l.limiterFor(c.ClientIP()).Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"success": false, "error": "too many requests",
			})
			return
		}
		c.Next()
	}
}
