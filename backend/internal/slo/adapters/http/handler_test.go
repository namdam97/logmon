package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	slohttp "github.com/namdam97/logmon/backend/internal/slo/adapters/http"
	"github.com/namdam97/logmon/backend/internal/slo/app/command"
	"github.com/namdam97/logmon/backend/internal/slo/app/query"
	"github.com/namdam97/logmon/backend/internal/slo/domain"
)

func init() { gin.SetMode(gin.TestMode) }

const ws = "00000000-0000-0000-0000-000000000001"

type stubCreator struct {
	got command.CreateSLOInput
	res domain.SLO
	err error
}

func (s *stubCreator) Handle(_ context.Context, in command.CreateSLOInput) (domain.SLO, error) {
	s.got = in
	return s.res, s.err
}

type stubUpdater struct{ err error }

func (s *stubUpdater) Handle(_ context.Context, _ command.UpdateSLOInput) (domain.SLO, error) {
	return domain.SLO{}, s.err
}

type stubDeleter struct{ err error }

func (s *stubDeleter) Handle(_ context.Context, _, _ string) error { return s.err }

type stubQueries struct {
	getErr error
	slo    domain.SLO
}

func (s *stubQueries) Get(_ context.Context, _, _ string) (domain.SLO, error) {
	return s.slo, s.getErr
}
func (s *stubQueries) List(_ context.Context, _ string) ([]domain.SLO, error) { return nil, nil }
func (s *stubQueries) Budget(_ context.Context, _, _ string) (query.BudgetView, error) {
	return query.BudgetView{}, s.getErr
}
func (s *stubQueries) Compliance(_ context.Context, _ string) ([]query.BudgetView, error) {
	return nil, nil
}

func newRouter(c slohttp.Handler) *gin.Engine {
	r := gin.New()
	c.Register(r.Group("/api/v1"), func(ctx *gin.Context) { ctx.Next() })
	return r
}

func mkSLO(t *testing.T) domain.SLO {
	t.Helper()
	id, _ := domain.NewSLOID("slo-1")
	s, err := domain.NewSLO(domain.NewSLOInput{
		ID: id, WorkspaceID: ws, Name: "checkout", Service: "checkout",
		SLIType: domain.SLIAvailability, Target: 0.999, WindowDays: 28,
		CreatedAt: time.Date(2026, 6, 27, 0, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	return s
}

func TestCreateSLO201(t *testing.T) {
	creator := &stubCreator{res: mkSLO(t)}
	h := slohttp.NewHandler(creator, &stubUpdater{}, &stubDeleter{}, &stubQueries{}, ws)
	r := newRouter(*h)

	body := `{"name":"checkout","service":"checkout","sliType":"availability","target":0.999,"windowDays":28}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/slos", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	require.Equal(t, "checkout", creator.got.Service)
	require.Equal(t, ws, creator.got.WorkspaceID)
}

func TestCreateSLOValidation400(t *testing.T) {
	creator := &stubCreator{err: &domain.ValidationError{Field: "target", Message: "bad"}}
	h := slohttp.NewHandler(creator, &stubUpdater{}, &stubDeleter{}, &stubQueries{}, ws)
	r := newRouter(*h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/slos", strings.NewReader(`{"name":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetSLONotFound404(t *testing.T) {
	h := slohttp.NewHandler(&stubCreator{}, &stubUpdater{}, &stubDeleter{}, &stubQueries{getErr: domain.ErrSLONotFound}, ws)
	r := newRouter(*h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/slos/slo-1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetSLO200(t *testing.T) {
	h := slohttp.NewHandler(&stubCreator{}, &stubUpdater{}, &stubDeleter{}, &stubQueries{slo: mkSLO(t)}, ws)
	r := newRouter(*h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/slos/slo-1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var env struct {
		Data struct {
			ID      string `json:"id"`
			SLIType string `json:"sliType"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.Equal(t, "slo-1", env.Data.ID)
	require.Equal(t, "availability", env.Data.SLIType)
}
