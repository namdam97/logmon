package http

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/alerting/app/command"
	"github.com/namdam97/logmon/backend/internal/alerting/domain"
)

type fakeIngester struct {
	res command.IngestResult
	err error
	got command.IngestWebhookInput
}

func (f *fakeIngester) Handle(_ context.Context, in command.IngestWebhookInput) (command.IngestResult, error) {
	f.got = in
	return f.res, f.err
}

type fakeInstanceReader struct {
	instances []domain.AlertInstance
	err       error
}

func (f *fakeInstanceReader) ListActive(context.Context, string) ([]domain.AlertInstance, error) {
	return f.instances, f.err
}

func noopMW(c *gin.Context) { c.Next() }

func newInstanceEngine(ing *fakeIngester, reader *fakeInstanceReader) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewInstanceHandler(ing, reader, testWorkspace)
	h.Register(r.Group("/api/v1"), noopMW, noopMW)
	return r
}

func TestWebhook_IngestsAlerts(t *testing.T) {
	ing := &fakeIngester{res: command.IngestResult{Firing: 1, Resolved: 1}}
	r := newInstanceEngine(ing, &fakeInstanceReader{})

	body := `{"alerts":[
		{"status":"firing","fingerprint":"aaaa1111","startsAt":"2026-01-01T00:00:00Z","labels":{"alertname":"HighErrorRate"}},
		{"status":"resolved","fingerprint":"bbbb2222","startsAt":"2026-01-01T00:00:00Z","endsAt":"2026-01-01T01:00:00Z","labels":{"alertname":"DiskFull"}}
	]}`
	rec := doJSON(t, r, http.MethodPost, "/api/v1/alerts/webhook", body)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, ing.got.Alerts, 2)
	require.Equal(t, "aaaa1111", ing.got.Alerts[0].Fingerprint)
	require.Equal(t, "firing", ing.got.Alerts[0].Status)
	require.Equal(t, testWorkspace, ing.got.WorkspaceID)
}

func TestWebhook_InvalidPayloadReturns400(t *testing.T) {
	ing := &fakeIngester{}
	r := newInstanceEngine(ing, &fakeInstanceReader{})

	rec := doJSON(t, r, http.MethodPost, "/api/v1/alerts/webhook", `{not json`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestWebhook_ValidationErrorReturns400(t *testing.T) {
	ing := &fakeIngester{err: &domain.ValidationError{Field: "fingerprint", Message: "must not be empty"}}
	r := newInstanceEngine(ing, &fakeInstanceReader{})

	rec := doJSON(t, r, http.MethodPost, "/api/v1/alerts/webhook",
		`{"alerts":[{"status":"firing","fingerprint":"","startsAt":"2026-01-01T00:00:00Z"}]}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestListActive_ReturnsInstances(t *testing.T) {
	fp, err := domain.NewFingerprint("aaaa1111")
	require.NoError(t, err)
	inst, err := domain.NewFiringInstance(domain.NewFiringInstanceInput{
		ID:          "11111111-1111-1111-1111-111111111111",
		WorkspaceID: testWorkspace,
		Fingerprint: fp,
		FiredAt:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Labels:      map[string]string{"alertname": "HighErrorRate"},
	})
	require.NoError(t, err)
	r := newInstanceEngine(&fakeIngester{}, &fakeInstanceReader{instances: []domain.AlertInstance{inst}})

	rec := doJSON(t, r, http.MethodGet, "/api/v1/alerts/active", "")

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Success bool               `json:"success"`
		Data    []instanceResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Len(t, resp.Data, 1)
	require.Equal(t, "firing", resp.Data[0].Status)
	require.Equal(t, "aaaa1111", resp.Data[0].Fingerprint)
	require.Empty(t, resp.Data[0].ResolvedAt)
}
