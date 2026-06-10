package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/shared/auth"
)

func init() { gin.SetMode(gin.TestMode) }

func TestNewJWTServiceValidation(t *testing.T) {
	tests := []struct {
		name    string
		secret  string
		ttl     time.Duration
		wantErr bool
	}{
		{name: "valid", secret: "s3cret", ttl: time.Hour},
		{name: "empty secret", secret: "", ttl: time.Hour, wantErr: true},
		{name: "non-positive ttl", secret: "s3cret", ttl: 0, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := auth.NewJWTService(tt.secret, tt.ttl)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestIssueParseRoundTrip(t *testing.T) {
	svc, err := auth.NewJWTService("s3cret", time.Hour)
	require.NoError(t, err)

	token, err := svc.Issue("user-1")
	require.NoError(t, err)
	require.NotEmpty(t, token)

	got, err := svc.Parse(token)
	require.NoError(t, err)
	require.Equal(t, "user-1", got)
}

func TestParseRejectsInvalid(t *testing.T) {
	svc, _ := auth.NewJWTService("s3cret", time.Hour)
	other, _ := auth.NewJWTService("different", time.Hour)

	good, _ := svc.Issue("user-1")
	expired, _ := mustExpired(t)

	tests := []struct {
		name string
		give string
	}{
		{name: "garbage", give: "not-a-token"},
		{name: "wrong secret", give: signWith(t, other, "user-1")},
		{name: "empty", give: ""},
		{name: "expired", give: expired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Parse(tt.give)
			require.ErrorIs(t, err, auth.ErrInvalidToken)
		})
	}
	_ = good
}

func TestRequireAuth(t *testing.T) {
	svc, _ := auth.NewJWTService("s3cret", time.Hour)
	token, _ := svc.Issue("user-42")

	build := func() *gin.Engine {
		r := gin.New()
		r.GET("/protected", auth.RequireAuth(svc), func(c *gin.Context) {
			id, ok := auth.UserIDFromContext(c)
			require.True(t, ok)
			c.String(http.StatusOK, id)
		})
		return r
	}

	t.Run("valid cookie passes", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		w := httptest.NewRecorder()
		build().ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, "user-42", w.Body.String())
	})

	t.Run("missing cookie is 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		w := httptest.NewRecorder()
		build().ServeHTTP(w, req)
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid cookie is 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: "bad"})
		w := httptest.NewRecorder()
		build().ServeHTTP(w, req)
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// helpers

func signWith(t *testing.T, svc *auth.JWTService, id string) string {
	t.Helper()
	tok, err := svc.Issue(id)
	require.NoError(t, err)
	return tok
}

// mustExpired tạo một service ttl cực ngắn rồi chờ token hết hạn.
func mustExpired(t *testing.T) (string, error) {
	t.Helper()
	svc, err := auth.NewJWTService("s3cret", time.Nanosecond)
	require.NoError(t, err)
	tok, err := svc.Issue("user-1")
	require.NoError(t, err)
	time.Sleep(2 * time.Millisecond)
	return tok, nil
}
