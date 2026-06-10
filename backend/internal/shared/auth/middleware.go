package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CookieName là tên cookie chứa access token.
const CookieName = "logmon_token"

const contextUserIDKey = "auth_user_id"

// TokenParser xác thực token và trả về userID. JWTService thoả interface này.
type TokenParser interface {
	Parse(raw string) (string, error)
}

// RequireAuth chặn request không có token hợp lệ trong cookie, trả 401. Khi hợp
// lệ, gắn userID vào gin.Context để handler dưới dùng.
func RequireAuth(parser TokenParser) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, err := c.Cookie(CookieName)
		if err != nil || raw == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false, "error": "unauthorized",
			})
			return
		}
		userID, err := parser.Parse(raw)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false, "error": "unauthorized",
			})
			return
		}
		c.Set(contextUserIDKey, userID)
		c.Next()
	}
}

// UserIDFromContext lấy userID đã xác thực; ok=false nếu request chưa qua RequireAuth.
func UserIDFromContext(c *gin.Context) (string, bool) {
	v, ok := c.Get(contextUserIDKey)
	if !ok {
		return "", false
	}
	id, ok := v.(string)
	return id, ok
}
