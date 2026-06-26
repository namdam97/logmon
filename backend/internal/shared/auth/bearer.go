package auth

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const _bearerPrefix = "Bearer "

// RequireBearerToken bảo vệ endpoint nội bộ (vd: Alertmanager webhook) bằng một
// bearer token chia sẻ. Fail-closed: token rỗng (chưa cấu hình) → mọi request bị
// từ chối. So sánh hằng-thời-gian chống timing attack. KHÔNG leak lý do cụ thể.
func RequireBearerToken(token string) gin.HandlerFunc {
	want := []byte(token)
	return func(c *gin.Context) {
		if len(want) == 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "error": "unauthorized"})
			return
		}
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, _bearerPrefix) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "error": "unauthorized"})
			return
		}
		got := []byte(strings.TrimPrefix(header, _bearerPrefix))
		if subtle.ConstantTimeCompare(got, want) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "error": "unauthorized"})
			return
		}
		c.Next()
	}
}
