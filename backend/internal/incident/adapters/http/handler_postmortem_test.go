package http

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/incident/app/command"
	"github.com/namdam97/logmon/backend/internal/incident/domain"
)

type stubPostmortemHandler struct {
	pm      domain.Postmortem
	item    domain.ActionItem
	err     error
	itemErr error
}

func (s stubPostmortemHandler) Submit(context.Context, command.SubmitPostmortemInput) (domain.Postmortem, error) {
	return s.pm, s.err
}
func (s stubPostmortemHandler) Publish(context.Context, string, string) (domain.Postmortem, error) {
	return s.pm, s.err
}
func (s stubPostmortemHandler) AddActionItem(context.Context, command.AddActionItemInput) (domain.ActionItem, error) {
	return s.item, s.itemErr
}
func (s stubPostmortemHandler) UpdateActionItemStatus(context.Context, string, string, string, string) (domain.ActionItem, error) {
	return s.item, s.itemErr
}

type stubPostmortemQueries struct {
	pm    domain.Postmortem
	items []domain.ActionItem
	err   error
}

func (s stubPostmortemQueries) GetByIncident(context.Context, string, string) (domain.Postmortem, []domain.ActionItem, error) {
	return s.pm, s.items, s.err
}

func samplePostmortem(t *testing.T) domain.Postmortem {
	t.Helper()
	id, err := domain.NewIncidentID("inc-1")
	require.NoError(t, err)
	pm, err := domain.NewPostmortem(domain.NewPostmortemInput{
		ID: "pm-1", IncidentID: id, WorkspaceID: _ws, RootCause: "root",
		Now: time.Unix(0, 0).UTC(),
	})
	require.NoError(t, err)
	return pm
}

func sampleActionItem(t *testing.T) domain.ActionItem {
	t.Helper()
	pid, err := domain.NewPostmortemID("pm-1")
	require.NoError(t, err)
	item, err := domain.NewActionItem(domain.NewActionItemInput{
		ID: "ai-1", PostmortemID: pid, Title: "fix", Assignee: "alice", Now: time.Unix(0, 0).UTC(),
	})
	require.NoError(t, err)
	return item
}

func newPMRouter(h *PostmortemHandler) *gin.Engine {
	r := gin.New()
	api := r.Group("/api/v1")
	h.Register(api, func(c *gin.Context) { c.Next() })
	return r
}

func TestSubmitPostmortemHTTP(t *testing.T) {
	h := NewPostmortemHandler(stubPostmortemHandler{pm: samplePostmortem(t)}, stubPostmortemQueries{}, _ws)
	w := doJSON(t, newPMRouter(h), http.MethodPost, "/api/v1/incidents/inc-1/postmortem",
		`{"rootCause":"db pool","lessonsLearned":"add alert","impact":{"durationSeconds":1800}}`)
	require.Equal(t, http.StatusOK, w.Code)

	var env struct {
		Data postmortemResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.Equal(t, "draft", env.Data.Status)
}

func TestGetPostmortemHTTP(t *testing.T) {
	h := NewPostmortemHandler(stubPostmortemHandler{}, stubPostmortemQueries{
		pm: samplePostmortem(t), items: []domain.ActionItem{sampleActionItem(t)},
	}, _ws)
	w := doJSON(t, newPMRouter(h), http.MethodGet, "/api/v1/incidents/inc-1/postmortem", "")
	require.Equal(t, http.StatusOK, w.Code)

	var env struct {
		Data postmortemDetailResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.Len(t, env.Data.ActionItems, 1)
	require.Equal(t, "fix", env.Data.ActionItems[0].Title)
}

func TestGetPostmortemNotFound(t *testing.T) {
	h := NewPostmortemHandler(stubPostmortemHandler{}, stubPostmortemQueries{err: domain.ErrPostmortemNotFound}, _ws)
	w := doJSON(t, newPMRouter(h), http.MethodGet, "/api/v1/incidents/inc-1/postmortem", "")
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestPublishPostmortemConflict(t *testing.T) {
	h := NewPostmortemHandler(stubPostmortemHandler{err: domain.ErrPostmortemPublished}, stubPostmortemQueries{}, _ws)
	w := doJSON(t, newPMRouter(h), http.MethodPost, "/api/v1/incidents/inc-1/postmortem/publish", "")
	require.Equal(t, http.StatusConflict, w.Code)
}

func TestAddActionItemHTTP(t *testing.T) {
	h := NewPostmortemHandler(stubPostmortemHandler{item: sampleActionItem(t)}, stubPostmortemQueries{}, _ws)
	w := doJSON(t, newPMRouter(h), http.MethodPost, "/api/v1/incidents/inc-1/postmortem/action-items",
		`{"title":"fix","assignee":"alice","dueDate":"2026-07-01T00:00:00Z"}`)
	require.Equal(t, http.StatusCreated, w.Code)
}

func TestAddActionItemBadDueDate(t *testing.T) {
	h := NewPostmortemHandler(stubPostmortemHandler{item: sampleActionItem(t)}, stubPostmortemQueries{}, _ws)
	w := doJSON(t, newPMRouter(h), http.MethodPost, "/api/v1/incidents/inc-1/postmortem/action-items",
		`{"title":"fix","dueDate":"nope"}`)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateActionItemHTTP(t *testing.T) {
	done := sampleActionItem(t).UpdateStatus(domain.ActionDone, time.Unix(100, 0).UTC())
	h := NewPostmortemHandler(stubPostmortemHandler{item: done}, stubPostmortemQueries{}, _ws)
	w := doJSON(t, newPMRouter(h), http.MethodPatch, "/api/v1/incidents/inc-1/postmortem/action-items/ai-1",
		`{"status":"done"}`)
	require.Equal(t, http.StatusOK, w.Code)

	var env struct {
		Data actionItemResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.Equal(t, "done", env.Data.Status)
}
