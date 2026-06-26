package alertmanager_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/alerting/adapters/alertmanager"
	"github.com/namdam97/logmon/backend/internal/alerting/domain"
)

func newSilence(t *testing.T) domain.Silence {
	t.Helper()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s, err := domain.NewSilence(domain.NewSilenceInput{
		Matchers:  []domain.MatcherInput{{Name: "alertname", Value: "HighErrorRate", IsEqual: true}},
		StartsAt:  start,
		EndsAt:    start.Add(2 * time.Hour),
		CreatedBy: "user-1",
		Comment:   "điều tra",
	})
	require.NoError(t, err)
	return s
}

func TestClient_CreateReturnsSilenceID(t *testing.T) {
	var gotBody postableSilenceWire
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/v2/silences", r.URL.Path)
		b, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(b, &gotBody))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"silenceID":"sil-123"}`))
	}))
	defer srv.Close()

	id, err := alertmanager.NewClient(srv.URL).Create(context.Background(), newSilence(t))

	require.NoError(t, err)
	require.Equal(t, "sil-123", id)
	require.Len(t, gotBody.Matchers, 1)
	require.Equal(t, "alertname", gotBody.Matchers[0].Name)
	require.True(t, gotBody.Matchers[0].IsEqual)
	require.Equal(t, "user-1", gotBody.CreatedBy)
}

func TestClient_CreateNon200Errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	_, err := alertmanager.NewClient(srv.URL).Create(context.Background(), newSilence(t))

	require.Error(t, err)
}

func TestClient_DeleteUsesSingularPath(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodDelete, r.Method)
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	err := alertmanager.NewClient(srv.URL).Delete(context.Background(), "sil-123")

	require.NoError(t, err)
	require.Equal(t, "/api/v2/silence/sil-123", gotPath)
}

func TestClient_DeleteNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	err := alertmanager.NewClient(srv.URL).Delete(context.Background(), "nope")

	require.True(t, errors.Is(err, domain.ErrSilenceNotFound))
}

func TestClient_ListMapsToView(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/v2/silences", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[
			{"id":"sil-1","status":{"state":"active"},"createdBy":"user-1","comment":"c",
			 "startsAt":"2026-01-01T00:00:00Z","endsAt":"2026-01-01T02:00:00Z",
			 "matchers":[{"name":"alertname","value":"HighErrorRate","isRegex":false,"isEqual":true}]}
		]`))
	}))
	defer srv.Close()

	views, err := alertmanager.NewClient(srv.URL).List(context.Background())

	require.NoError(t, err)
	require.Len(t, views, 1)
	require.Equal(t, "sil-1", views[0].ID)
	require.Equal(t, "active", views[0].Status)
	require.Len(t, views[0].Matchers, 1)
	require.Equal(t, "alertname", views[0].Matchers[0].Name())
	require.True(t, views[0].Matchers[0].IsEqual())
}

// postableSilenceWire phản chiếu body POST để assert (test ở package _test nên
// không truy cập struct nội bộ của adapter).
type postableSilenceWire struct {
	Matchers []struct {
		Name    string `json:"name"`
		Value   string `json:"value"`
		IsRegex bool   `json:"isRegex"`
		IsEqual bool   `json:"isEqual"`
	} `json:"matchers"`
	CreatedBy string `json:"createdBy"`
	Comment   string `json:"comment"`
}
