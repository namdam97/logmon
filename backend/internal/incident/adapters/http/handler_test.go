package http

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

	"github.com/namdam97/logmon/backend/internal/incident/app/command"
	"github.com/namdam97/logmon/backend/internal/incident/domain"
)

func init() { gin.SetMode(gin.TestMode) }

const _ws = "ws-1"

func sampleIncident(t *testing.T, status domain.Status) domain.Incident {
	t.Helper()
	id, err := domain.NewIncidentID("11111111-1111-1111-1111-111111111111")
	require.NoError(t, err)
	return domain.Reconstruct(domain.ReconstructInput{
		ID: id, WorkspaceID: _ws, Title: "db down", Service: "orders",
		Severity: domain.SEV1, Status: status, Source: domain.SourceManual,
		CreatedAt: time.Unix(0, 0).UTC(), UpdatedAt: time.Unix(0, 0).UTC(),
	})
}

type stubCreator struct {
	inc domain.Incident
	err error
}

func (s stubCreator) Handle(context.Context, command.CreateIncidentInput) (domain.Incident, error) {
	return s.inc, s.err
}

type stubTransitions struct {
	inc domain.Incident
	err error
}

func (s stubTransitions) Triage(context.Context, string, string, string, string, string) (domain.Incident, error) {
	return s.inc, s.err
}
func (s stubTransitions) Assign(context.Context, string, string, string, string) (domain.Incident, error) {
	return s.inc, s.err
}
func (s stubTransitions) StartMitigation(context.Context, string, string, string) (domain.Incident, error) {
	return s.inc, s.err
}
func (s stubTransitions) Resolve(context.Context, string, string, string, string) (domain.Incident, error) {
	return s.inc, s.err
}
func (s stubTransitions) RequirePostmortem(context.Context, string, string, string) (domain.Incident, error) {
	return s.inc, s.err
}
func (s stubTransitions) Close(context.Context, string, string, string, string) (domain.Incident, error) {
	return s.inc, s.err
}

type stubQueries struct {
	inc       domain.Incident
	incidents []domain.Incident
	entries   []domain.TimelineEntry
	err       error
}

func (s stubQueries) Get(context.Context, string, string) (domain.Incident, error) {
	return s.inc, s.err
}
func (s stubQueries) List(context.Context, string) ([]domain.Incident, error) {
	return s.incidents, s.err
}
func (s stubQueries) ListActive(context.Context, string) ([]domain.Incident, error) {
	return s.incidents, s.err
}
func (s stubQueries) Timeline(context.Context, string, string) ([]domain.TimelineEntry, error) {
	return s.entries, s.err
}

func newRouter(h *Handler) *gin.Engine {
	r := gin.New()
	api := r.Group("/api/v1")
	h.Register(api, func(c *gin.Context) { c.Next() })
	return r
}

func doJSON(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestCreateIncidentHTTP(t *testing.T) {
	h := NewHandler(stubCreator{inc: sampleIncident(t, domain.StatusOpen)}, stubTransitions{}, stubQueries{}, _ws)
	w := doJSON(t, newRouter(h), http.MethodPost, "/api/v1/incidents",
		`{"title":"db down","service":"orders"}`)
	require.Equal(t, http.StatusCreated, w.Code)

	var env struct {
		Data incidentResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.Equal(t, "open", env.Data.Status)
	require.Equal(t, "SEV1", env.Data.Severity)
}

func TestInvalidTransitionReturns409(t *testing.T) {
	h := NewHandler(stubCreator{}, stubTransitions{err: domain.ErrInvalidTransition}, stubQueries{}, _ws)
	w := doJSON(t, newRouter(h), http.MethodPost, "/api/v1/incidents/abc/assign", `{"assignee":"alice"}`)
	require.Equal(t, http.StatusConflict, w.Code)
}

func TestNotFoundReturns404(t *testing.T) {
	h := NewHandler(stubCreator{}, stubTransitions{}, stubQueries{err: domain.ErrIncidentNotFound}, _ws)
	w := doJSON(t, newRouter(h), http.MethodGet, "/api/v1/incidents/abc", "")
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestValidationReturns400(t *testing.T) {
	ve := &domain.ValidationError{Field: "severity", Message: "bad"}
	h := NewHandler(stubCreator{}, stubTransitions{err: ve}, stubQueries{}, _ws)
	w := doJSON(t, newRouter(h), http.MethodPost, "/api/v1/incidents/abc/triage", `{"severity":"SEV9"}`)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestListAndTimeline(t *testing.T) {
	inc := sampleIncident(t, domain.StatusOpen)
	entry, err := domain.NewTimelineEntry(domain.NewTimelineEntryInput{
		ID: "e1", IncidentID: inc.ID().String(), Kind: domain.KindCreated,
		ToStatus: domain.StatusOpen, Actor: "op", At: time.Unix(0, 0).UTC(),
	})
	require.NoError(t, err)
	h := NewHandler(stubCreator{}, stubTransitions{}, stubQueries{
		incidents: []domain.Incident{inc}, entries: []domain.TimelineEntry{entry},
	}, _ws)
	r := newRouter(h)

	w := doJSON(t, r, http.MethodGet, "/api/v1/incidents?active=true", "")
	require.Equal(t, http.StatusOK, w.Code)

	w = doJSON(t, r, http.MethodGet, "/api/v1/incidents/abc/timeline", "")
	require.Equal(t, http.StatusOK, w.Code)
	var env struct {
		Data []timelineResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.Len(t, env.Data, 1)
	require.Equal(t, "created", env.Data[0].Kind)
}
