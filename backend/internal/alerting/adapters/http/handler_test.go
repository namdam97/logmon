package http

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

	"github.com/namdam97/logmon/backend/internal/alerting/app/command"
	"github.com/namdam97/logmon/backend/internal/alerting/domain"
)

const testWorkspace = "00000000-0000-0000-0000-000000000001"

type fakeCreator struct {
	rule   domain.AlertRule
	err    error
	got    command.CreateRuleInput
	called bool
}

func (f *fakeCreator) Handle(_ context.Context, in command.CreateRuleInput) (domain.AlertRule, error) {
	f.called = true
	f.got = in
	return f.rule, f.err
}

type fakeReader struct {
	rule  domain.AlertRule
	rules []domain.AlertRule
	err   error
}

func (f *fakeReader) Get(context.Context, string) (domain.AlertRule, error) {
	return f.rule, f.err
}

func (f *fakeReader) List(context.Context, string) ([]domain.AlertRule, error) {
	return f.rules, f.err
}

func fixtureRule(t *testing.T) domain.AlertRule {
	t.Helper()
	id, err := domain.NewRuleID("11111111-1111-1111-1111-111111111111")
	require.NoError(t, err)
	sev, err := domain.NewSeverity("critical")
	require.NoError(t, err)
	ts := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)
	return domain.Reconstruct(domain.ReconstructInput{
		ID:          id,
		WorkspaceID: testWorkspace,
		Name:        "HighErrorRate",
		Expression:  "rate(http_errors_total[5m]) > 0.05",
		Service:     "api",
		ForDuration: 2 * time.Minute,
		Severity:    sev,
		Labels:      map[string]string{"team": "sre"},
		Annotations: map[string]string{"summary": "errors high", "runbook_url": "http://rb/x"},
		Enabled:     true,
		SyncStatus:  domain.SyncPending,
		CreatedAt:   ts,
		UpdatedAt:   ts,
	})
}

func newEngine(creator ruleCreator, reader ruleReader) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewHandler(creator, reader, testWorkspace)
	passthrough := func(c *gin.Context) { c.Next() }
	h.Register(r.Group("/api/v1"), passthrough)
	return r
}

func doJSON(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestCreateRule(t *testing.T) {
	const validBody = `{"name":"HighErrorRate","expression":"rate(http_errors_total[5m]) > 0.05",` +
		`"service":"api","severity":"critical","forDuration":"2m",` +
		`"annotations":{"summary":"s","runbook_url":"http://rb"}}`

	tests := []struct {
		name       string
		body       string
		creatorErr error
		wantStatus int
		wantCalled bool
	}{
		{name: "valid", body: validBody, wantStatus: http.StatusCreated, wantCalled: true},
		{name: "invalid json", body: "{", wantStatus: http.StatusBadRequest},
		{
			name:       "missing expression",
			body:       `{"name":"x","service":"api","severity":"critical","forDuration":"2m","annotations":{"summary":"s"}}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "bad severity",
			body:       `{"name":"x","expression":"up","service":"api","severity":"fatal","forDuration":"2m","annotations":{"summary":"s"}}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "bad duration",
			body:       `{"name":"x","expression":"up","service":"api","severity":"critical","forDuration":"abc","annotations":{"summary":"s"}}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "domain validation error",
			body:       validBody,
			creatorErr: &domain.ValidationError{Field: "expression", Message: "bad promql"},
			wantStatus: http.StatusBadRequest,
			wantCalled: true,
		},
		{name: "name taken", body: validBody, creatorErr: domain.ErrRuleNameTaken, wantStatus: http.StatusConflict, wantCalled: true},
		{name: "internal error", body: validBody, creatorErr: errors.New("boom"), wantStatus: http.StatusInternalServerError, wantCalled: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creator := &fakeCreator{rule: fixtureRule(t), err: tt.creatorErr}
			r := newEngine(creator, &fakeReader{})

			w := doJSON(t, r, http.MethodPost, "/api/v1/alert-rules", tt.body)

			require.Equal(t, tt.wantStatus, w.Code)
			require.Equal(t, tt.wantCalled, creator.called)
			if tt.wantStatus == http.StatusCreated {
				var env struct {
					Success bool         `json:"success"`
					Data    ruleResponse `json:"data"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
				require.True(t, env.Success)
				require.Equal(t, "HighErrorRate", env.Data.Name)
				require.Equal(t, testWorkspace, creator.got.WorkspaceID)
				require.Equal(t, 2*time.Minute, creator.got.ForDuration)
			}
		})
	}
}

func TestGetRule(t *testing.T) {
	tests := []struct {
		name       string
		readerErr  error
		wantStatus int
	}{
		{name: "found", wantStatus: http.StatusOK},
		{name: "not found", readerErr: domain.ErrRuleNotFound, wantStatus: http.StatusNotFound},
		{name: "internal error", readerErr: errors.New("boom"), wantStatus: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &fakeReader{rule: fixtureRule(t), err: tt.readerErr}
			r := newEngine(&fakeCreator{}, reader)

			w := doJSON(t, r, http.MethodGet, "/api/v1/alert-rules/11111111-1111-1111-1111-111111111111", "")

			require.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestListRules(t *testing.T) {
	reader := &fakeReader{rules: []domain.AlertRule{fixtureRule(t)}}
	r := newEngine(&fakeCreator{}, reader)

	w := doJSON(t, r, http.MethodGet, "/api/v1/alert-rules", "")

	require.Equal(t, http.StatusOK, w.Code)
	var env struct {
		Success bool           `json:"success"`
		Data    []ruleResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.True(t, env.Success)
	require.Len(t, env.Data, 1)
	require.Equal(t, "HighErrorRate", env.Data[0].Name)
}
