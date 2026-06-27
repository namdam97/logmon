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

// authenticate xác thực token cookie và gắn userID vào context. Trả false +
// abort 401 nếu thất bại. Tách riêng để compose với middleware khác mà không
// gọi c.Next() sớm (xem RequireAuthWorkspace).
func authenticate(c *gin.Context, parser TokenParser) bool {
	raw, err := c.Cookie(CookieName)
	if err != nil || raw == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "error": "unauthorized"})
		return false
	}
	userID, err := parser.Parse(raw)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "error": "unauthorized"})
		return false
	}
	c.Set(contextUserIDKey, userID)
	return true
}

// RequireAuth chặn request không có token hợp lệ trong cookie, trả 401. Khi hợp
// lệ, gắn userID vào gin.Context để handler dưới dùng.
func RequireAuth(parser TokenParser) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !authenticate(c, parser) {
			return
		}
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
