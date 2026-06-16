package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/shared/auth"
	"github.com/namdam97/logmon/backend/internal/shared/httpx"
	"github.com/namdam97/logmon/backend/internal/shared/middleware"
	userhttp "github.com/namdam97/logmon/backend/internal/user/adapters/http"
	"github.com/namdam97/logmon/backend/internal/user/app"
	"github.com/namdam97/logmon/backend/internal/user/domain"
)

// --- test doubles cho ports, đủ để dựng app.Service thật ---

type fakeRepo struct {
	byID    map[string]domain.User
	byEmail map[string]domain.User
}

func (r *fakeRepo) Save(_ context.Context, u domain.User) error {
	if _, ok := r.byEmail[u.Email().String()]; ok {
		return domain.ErrEmailTaken
	}
	r.byID[u.ID().String()] = u
	r.byEmail[u.Email().String()] = u
	return nil
}

func (r *fakeRepo) ByID(_ context.Context, id domain.UserID) (domain.User, error) {
	u, ok := r.byID[id.String()]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound
	}
	return u, nil
}

func (r *fakeRepo) ByEmail(_ context.Context, email domain.Email) (domain.User, error) {
	u, ok := r.byEmail[email.String()]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound
	}
	return u, nil
}

type fakeHasher struct{}

func (fakeHasher) Hash(p string) (string, error) { return "h:" + p, nil }

func (fakeHasher) Verify(hash, plain string) error {
	if hash == "h:"+plain {
		return nil
	}
	return errors.New("mismatch")
}

type fixedID struct{ id string }

func (g fixedID) NewID() string { return g.id }

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }

func newRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := &fakeRepo{byID: map[string]domain.User{}, byEmail: map[string]domain.User{}}
	jwtSvc, err := auth.NewJWTService("test-secret", time.Hour)
	require.NoError(t, err)
	svc := app.NewService(repo, fakeHasher{}, fixedID{id: "user-1"}, fixedClock{}, jwtSvc)

	r := gin.New()
	h := userhttp.NewHandler(svc, userhttp.CookieConfig{Secure: false, MaxAgeSeconds: 3600})
	// Rate limit cao để không cản test gọi nhiều request.
	rate := middleware.NewPerMinuteLimiter(100000, 1000)
	h.Register(r.Group("/api/v1"), auth.RequireAuth(jwtSvc), rate.Middleware())
	return r
}

func doJSON(t *testing.T, r http.Handler, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestRegisterEndpoint(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{name: "valid", body: `{"email":"a@b.com","password":"password123"}`, wantStatus: http.StatusCreated},
		{name: "invalid email", body: `{"email":"nope","password":"password123"}`, wantStatus: http.StatusBadRequest},
		{name: "short password", body: `{"email":"a@b.com","password":"x"}`, wantStatus: http.StatusBadRequest},
		{name: "malformed json", body: `{`, wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRouter(t)
			w := doJSON(t, r, http.MethodPost, "/api/v1/users", tt.body, nil)
			require.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestLoginEndpoint(t *testing.T) {
	r := newRouter(t)
	reg := doJSON(t, r, http.MethodPost, "/api/v1/users", `{"email":"a@b.com","password":"password123"}`, nil)
	require.Equal(t, http.StatusCreated, reg.Code)

	t.Run("valid login sets cookie", func(t *testing.T) {
		w := doJSON(t, r, http.MethodPost, "/api/v1/auth/login", `{"email":"a@b.com","password":"password123"}`, nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.NotEmpty(t, loginCookie(w), "expected auth cookie to be set")
	})

	t.Run("wrong password is 401", func(t *testing.T) {
		w := doJSON(t, r, http.MethodPost, "/api/v1/auth/login", `{"email":"a@b.com","password":"wrongpass"}`, nil)
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("unknown email is 401", func(t *testing.T) {
		w := doJSON(t, r, http.MethodPost, "/api/v1/auth/login", `{"email":"x@y.com","password":"password123"}`, nil)
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestProtectedRoutes(t *testing.T) {
	r := newRouter(t)
	reg := doJSON(t, r, http.MethodPost, "/api/v1/users", `{"email":"a@b.com","password":"password123"}`, nil)
	require.Equal(t, http.StatusCreated, reg.Code)
	id := extractID(t, reg)

	t.Run("get without cookie is 401", func(t *testing.T) {
		w := doJSON(t, r, http.MethodGet, "/api/v1/users/"+id, "", nil)
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	login := doJSON(t, r, http.MethodPost, "/api/v1/auth/login", `{"email":"a@b.com","password":"password123"}`, nil)
	cookie := loginCookie(login)
	require.NotNil(t, cookie)

	t.Run("get with cookie succeeds", func(t *testing.T) {
		w := doJSON(t, r, http.MethodGet, "/api/v1/users/"+id, "", cookie)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("get another user's id is 403", func(t *testing.T) {
		w := doJSON(t, r, http.MethodGet, "/api/v1/users/someone-else", "", cookie)
		require.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("me returns current user", func(t *testing.T) {
		w := doJSON(t, r, http.MethodGet, "/api/v1/me", "", cookie)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, id, extractID(t, w))
	})
}

func TestLogoutClearsCookie(t *testing.T) {
	r := newRouter(t)
	reg := doJSON(t, r, http.MethodPost, "/api/v1/users", `{"email":"a@b.com","password":"password123"}`, nil)
	require.Equal(t, http.StatusCreated, reg.Code)
	login := doJSON(t, r, http.MethodPost, "/api/v1/auth/login", `{"email":"a@b.com","password":"password123"}`, nil)
	require.NotNil(t, loginCookie(login))

	w := doJSON(t, r, http.MethodPost, "/api/v1/auth/logout", "", nil)
	require.Equal(t, http.StatusOK, w.Code)

	cleared := loginCookie(w)
	require.NotNil(t, cleared, "logout phải gửi Set-Cookie để xoá cookie")
	require.Empty(t, cleared.Value)
	require.True(t, cleared.MaxAge < 0, "cookie phải bị huỷ (MaxAge<0)")
}

func extractID(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var env httpx.Envelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	data, _ := env.Data.(map[string]any)
	id, _ := data["id"].(string)
	require.NotEmpty(t, id)
	return id
}

func loginCookie(w *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == auth.CookieName {
			return c
		}
	}
	return nil
}
