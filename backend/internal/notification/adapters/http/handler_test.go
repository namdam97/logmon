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

	"github.com/namdam97/logmon/backend/internal/notification/app/command"
	"github.com/namdam97/logmon/backend/internal/notification/domain"
)

func init() { gin.SetMode(gin.TestMode) }

type stubCreator struct {
	ch  domain.Channel
	err error
}

func (s stubCreator) Handle(context.Context, command.CreateChannelInput) (domain.Channel, error) {
	return s.ch, s.err
}

type stubUpdater struct{}

func (stubUpdater) Handle(context.Context, command.UpdateChannelInput) (domain.Channel, error) {
	return domain.Channel{}, nil
}

type stubDeleter struct{ err error }

func (s stubDeleter) Handle(context.Context, string, string) error { return s.err }

type stubTester struct{ err error }

func (s stubTester) Handle(context.Context, string, string) error { return s.err }

type stubReader struct {
	ch  domain.Channel
	err error
}

func (s stubReader) ByID(context.Context, string, domain.ChannelID) (domain.Channel, error) {
	return s.ch, s.err
}
func (s stubReader) List(context.Context, string) ([]domain.Channel, error) {
	return []domain.Channel{s.ch}, s.err
}

type stubHistory struct{ entries []domain.HistoryEntry }

func (s stubHistory) List(context.Context, string, int) ([]domain.HistoryEntry, error) {
	return s.entries, nil
}

func sampleChannel(t *testing.T) domain.Channel {
	t.Helper()
	id, err := domain.NewChannelID("11111111-1111-1111-1111-111111111111")
	require.NoError(t, err)
	c, err := domain.NewChannel(domain.NewChannelInput{
		ID: id, WorkspaceID: "ws-1", Name: "team slack", ChannelType: domain.ChannelSlack,
		Config: map[string]string{"webhook_url": "https://hooks.slack.com/SECRET-TOKEN"},
		Events: []string{domain.EventAlertFired}, CreatedAt: time.Unix(0, 0).UTC(),
	})
	require.NoError(t, err)
	return c
}

func newRouter(h *Handler) *gin.Engine {
	r := gin.New()
	api := r.Group("/api/v1")
	noAuth := func(c *gin.Context) { c.Set("auth_role", "admin"); c.Next() }
	h.Register(api, noAuth)
	return r
}

func TestCreateChannelRedactsSecret(t *testing.T) {
	h := NewHandler(stubCreator{ch: sampleChannel(t)}, stubUpdater{}, stubDeleter{}, stubTester{}, stubReader{}, stubHistory{}, "ws-1")
	r := newRouter(h)

	body := `{"name":"team slack","channelType":"slack","config":{"webhook_url":"https://hooks.slack.com/SECRET-TOKEN"},"events":["alert_fired"]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/channels", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	require.NotContains(t, w.Body.String(), "SECRET-TOKEN", "secret must not leak in response")
	require.Contains(t, w.Body.String(), "webhook_url", "config keys listed")
}

func TestListChannel(t *testing.T) {
	h := NewHandler(stubCreator{}, stubUpdater{}, stubDeleter{}, stubTester{}, stubReader{ch: sampleChannel(t)}, stubHistory{}, "ws-1")
	r := newRouter(h)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/notifications/channels", nil))

	require.Equal(t, http.StatusOK, w.Code)
	require.NotContains(t, w.Body.String(), "SECRET-TOKEN")
}

func TestDeleteChannelNotFound(t *testing.T) {
	h := NewHandler(stubCreator{}, stubUpdater{}, stubDeleter{err: domain.ErrChannelNotFound}, stubTester{}, stubReader{}, stubHistory{}, "ws-1")
	r := newRouter(h)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/v1/notifications/channels/x", nil))

	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestTestChannelFailureReturns502(t *testing.T) {
	h := NewHandler(stubCreator{}, stubUpdater{}, stubDeleter{}, stubTester{err: errStub("send failed")}, stubReader{}, stubHistory{}, "ws-1")
	r := newRouter(h)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/notifications/channels/x/test", nil))

	require.Equal(t, http.StatusBadGateway, w.Code)
}

func TestListHistory(t *testing.T) {
	entries := []domain.HistoryEntry{{ChannelID: "ch-1", EventType: "alert_fired", Status: domain.StatusSent, SentAt: time.Unix(0, 0).UTC()}}
	h := NewHandler(stubCreator{}, stubUpdater{}, stubDeleter{}, stubTester{}, stubReader{}, stubHistory{entries: entries}, "ws-1")
	r := newRouter(h)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/notifications/history", nil))

	require.Equal(t, http.StatusOK, w.Code)
	var env struct {
		Data []historyResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.Len(t, env.Data, 1)
	require.Equal(t, "sent", env.Data[0].Status)
}

type errStub string

func (e errStub) Error() string { return string(e) }
