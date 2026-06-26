package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/shared/auth"
)

const _csrfTestSecret = "csrf-test-secret-0123456789abcdef"

func newCSRFProtector(t *testing.T) *auth.CSRFProtector {
	t.Helper()
	p, err := auth.NewCSRFProtector(_csrfTestSecret)
	require.NoError(t, err)
	return p
}

func TestNewCSRFProtector_RejectsShortSecret(t *testing.T) {
	_, err := auth.NewCSRFProtector("short")
	require.Error(t, err)
}

func TestCSRFProtector_IssueProducesValidToken(t *testing.T) {
	p := newCSRFProtector(t)

	a, err := p.Issue()
	require.NoError(t, err)
	b, err := p.Issue()
	require.NoError(t, err)

	require.NotEmpty(t, a)
	require.NotEqual(t, a, b, "mỗi token phải ngẫu nhiên")
}

func newCSRFRouter(t *testing.T, p *auth.CSRFProtector, exempt ...string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(p.Middleware(exempt...))
	ok := func(c *gin.Context) { c.Status(http.StatusOK) }
	api.GET("/things", ok)
	api.POST("/things", ok)
	api.POST("/auth/login", ok)
	return r
}

func do(t *testing.T, r *gin.Engine, method, path, cookie, header string) int {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: auth.CSRFCookieName, Value: cookie})
	}
	if header != "" {
		req.Header.Set(auth.CSRFHeaderName, header)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

func TestCSRFMiddleware(t *testing.T) {
	p := newCSRFProtector(t)
	token, err := p.Issue()
	require.NoError(t, err)

	tests := []struct {
		name           string
		method, path   string
		cookie, header string
		want           int
	}{
		{name: "safe method skips", method: http.MethodGet, path: "/api/v1/things", want: http.StatusOK},
		{name: "missing both blocked", method: http.MethodPost, path: "/api/v1/things", want: http.StatusForbidden},
		{name: "missing header blocked", method: http.MethodPost, path: "/api/v1/things", cookie: token, want: http.StatusForbidden},
		{name: "missing cookie blocked", method: http.MethodPost, path: "/api/v1/things", header: token, want: http.StatusForbidden},
		{name: "mismatch blocked", method: http.MethodPost, path: "/api/v1/things", cookie: token, header: token + "x", want: http.StatusForbidden},
		{name: "bad signature blocked", method: http.MethodPost, path: "/api/v1/things", cookie: "abc.def", header: "abc.def", want: http.StatusForbidden},
		{name: "valid token allowed", method: http.MethodPost, path: "/api/v1/things", cookie: token, header: token, want: http.StatusOK},
		{name: "exempt path skips", method: http.MethodPost, path: "/api/v1/auth/login", want: http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newCSRFRouter(t, p, "/api/v1/auth/login")
			got := do(t, r, tt.method, tt.path, tt.cookie, tt.header)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestCSRFMiddleware_RejectsForgedSignature(t *testing.T) {
	p := newCSRFProtector(t)
	other, err := auth.NewCSRFProtector("a-totally-different-secret-value")
	require.NoError(t, err)
	forged, err := other.Issue() // chữ ký từ secret khác → không hợp lệ
	require.NoError(t, err)

	r := newCSRFRouter(t, p)
	got := do(t, r, http.MethodPost, "/api/v1/things", forged, forged)
	require.Equal(t, http.StatusForbidden, got)
}
