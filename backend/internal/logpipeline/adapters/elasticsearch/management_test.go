package elasticsearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/logpipeline/domain"
)

func TestManagementApplyILM(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := NewManagement(srv.URL, "", "")
	err := m.Apply(context.Background(), "default", domain.ILMPolicy{HotDays: 7, WarmDays: 30, DeleteDays: 90})
	require.NoError(t, err)
	require.Equal(t, http.MethodPut, gotMethod)
	require.Equal(t, "/_ilm/policy/logs-default-policy", gotPath)
}

func TestManagementCheck(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer up.Close()
	require.Equal(t, "up", NewManagement(up.URL, "", "").Check(context.Background()).Elasticsearch)

	require.Equal(t, "down", NewManagement("http://127.0.0.1:0", "", "").Check(context.Background()).Elasticsearch)
}

func TestManagementStats(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data_streams":[{"data_stream":"logs-api-default","backing_indices":3,"store_size_bytes":1024}]}`))
	}))
	defer srv.Close()

	stats, err := NewManagement(srv.URL, "", "").Stats(context.Background(), "default")
	require.NoError(t, err)
	require.Len(t, stats, 1)
	require.Equal(t, "logs-api-default", stats[0].Name)
	require.Equal(t, 3, stats[0].BackingIndices)
	require.Equal(t, int64(1024), stats[0].SizeBytes)
}
