package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/shared/httpx"
	userhttp "github.com/namdam97/logmon/backend/internal/user/adapters/http"
	"github.com/namdam97/logmon/backend/internal/user/app"
	"github.com/namdam97/logmon/backend/internal/user/domain"
)

// --- test doubles cho ports, đủ để dựng app.Service thật ---

type fakeRepo struct{ byID map[string]domain.User }

func (r *fakeRepo) Save(_ context.Context, u domain.User) error {
	if _, ok := r.byID[u.ID().String()]; ok {
		return domain.ErrEmailTaken
	}
	r.byID[u.ID().String()] = u
	return nil
}

func (r *fakeRepo) ByID(_ context.Context, id domain.UserID) (domain.User, error) {
	u, ok := r.byID[id.String()]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound
	}
	return u, nil
}

type fakeHasher struct{}

func (fakeHasher) Hash(p string) (string, error) { return "h:" + p, nil }

type fixedID struct{ id string }

func (g fixedID) NewID() string { return g.id }

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }

func newRouter() (*gin.Engine, *fakeRepo) {
	gin.SetMode(gin.TestMode)
	repo := &fakeRepo{byID: map[string]domain.User{}}
	svc := app.NewService(repo, fakeHasher{}, fixedID{id: "user-1"}, fixedClock{})
	r := gin.New()
	userhttp.NewHandler(svc).Register(r.Group("/api/v1"))
	return r, repo
}

func doJSON(t *testing.T, r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
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
			r, _ := newRouter()
			w := doJSON(t, r, http.MethodPost, "/api/v1/users", tt.body)
			require.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestGetEndpoint(t *testing.T) {
	r, _ := newRouter()
	created := doJSON(t, r, http.MethodPost, "/api/v1/users", `{"email":"a@b.com","password":"password123"}`)
	require.Equal(t, http.StatusCreated, created.Code)

	var env httpx.Envelope
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &env))
	data, _ := env.Data.(map[string]any)
	id, _ := data["id"].(string)
	require.NotEmpty(t, id)

	t.Run("found", func(t *testing.T) {
		w := doJSON(t, r, http.MethodGet, "/api/v1/users/"+id, "")
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("not found", func(t *testing.T) {
		w := doJSON(t, r, http.MethodGet, "/api/v1/users/missing", "")
		require.Equal(t, http.StatusNotFound, w.Code)
	})
}
