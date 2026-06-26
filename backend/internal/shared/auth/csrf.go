package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// CSRFCookieName chứa token CSRF; KHÔNG đặt HttpOnly để JS đọc và gửi lại qua
// header (double-submit). CSRFHeaderName là header client echo token về.
const (
	CSRFCookieName = "lm_csrf"
	CSRFHeaderName = "X-CSRF-Token"
)

const (
	_csrfRandomBytes  = 32
	_csrfMinSecretLen = 16
)

// CSRFProtector phát hành + xác thực token CSRF theo mô hình signed double-submit:
// token = <random-hex>.<hmac-hex>. HMAC (khoá server) chống giả mạo token kể cả
// khi attacker chèn được cookie qua subdomain — không có khoá thì không ký được.
type CSRFProtector struct {
	key []byte
}

// NewCSRFProtector tạo protector với secret (tối thiểu 16 byte) dùng làm khoá HMAC.
func NewCSRFProtector(secret string) (*CSRFProtector, error) {
	if len(secret) < _csrfMinSecretLen {
		return nil, fmt.Errorf("csrf secret must be at least %d bytes", _csrfMinSecretLen)
	}
	return &CSRFProtector{key: []byte(secret)}, nil
}

// Issue tạo token CSRF mới đã ký. Client lưu vào cookie lm_csrf rồi gửi lại qua
// header X-CSRF-Token cho mỗi request mutating.
func (p *CSRFProtector) Issue() (string, error) {
	raw := make([]byte, _csrfRandomBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("csrf rand: %w", err)
	}
	msg := hex.EncodeToString(raw)
	return msg + "." + p.sign(msg), nil
}

// sign trả về HMAC-SHA256(key, msg) dạng hex.
func (p *CSRFProtector) sign(msg string) string {
	mac := hmac.New(sha256.New, p.key)
	mac.Write([]byte(msg))
	return hex.EncodeToString(mac.Sum(nil))
}

// valid kiểm tra token có đúng định dạng <msg>.<sig> và chữ ký khớp khoá server.
func (p *CSRFProtector) valid(token string) bool {
	msg, sig, ok := strings.Cut(token, ".")
	if !ok || msg == "" || sig == "" {
		return false
	}
	return hmac.Equal([]byte(sig), []byte(p.sign(msg)))
}

// Middleware chặn request mutating (không phải safe method) nếu thiếu cookie/header
// CSRF, hai giá trị không khớp, hoặc chữ ký không hợp lệ → 403. exempt là danh sách
// FullPath bỏ qua kiểm tra (login/register/refresh/webhook — không dựa cookie session).
func (p *CSRFProtector) Middleware(exempt ...string) gin.HandlerFunc {
	skip := make(map[string]struct{}, len(exempt))
	for _, path := range exempt {
		skip[path] = struct{}{}
	}
	return func(c *gin.Context) {
		if isSafeMethod(c.Request.Method) {
			c.Next()
			return
		}
		if _, ok := skip[c.FullPath()]; ok {
			c.Next()
			return
		}
		cookie, err := c.Cookie(CSRFCookieName)
		header := c.GetHeader(CSRFHeaderName)
		if err != nil || cookie == "" || header == "" ||
			subtle.ConstantTimeCompare([]byte(cookie), []byte(header)) != 1 ||
			!p.valid(header) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false, "error": "forbidden",
			})
			return
		}
		c.Next()
	}
}

// isSafeMethod báo method không làm thay đổi trạng thái (RFC 7231) — miễn CSRF.
func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}
