package prometheus

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestIngestionBytes(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("query")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"value":[1700000000,"2147483648"]}]}}`))
	}))
	defer srv.Close()

	got, err := NewReader(srv.URL).IngestionBytes(context.Background(), "ws-1", time.Unix(0, 0).UTC())
	require.NoError(t, err)
	require.Equal(t, int64(2147483648), got)
	// matcher workspace được inject vào PromQL.
	require.Contains(t, gotQuery, `workspace="ws-1"`)
	require.Contains(t, gotQuery, "logmon_ingested_bytes_total")
}

func TestEmptyResultIsZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
	defer srv.Close()
	got, err := NewReader(srv.URL).StorageBytes(context.Background(), "ws-1")
	require.NoError(t, err)
	require.Equal(t, int64(0), got)
}

func TestPrometheusErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	_, err := NewReader(srv.URL).LogCount(context.Background(), "ws-1", time.Unix(0, 0).UTC())
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "prometheus status"))
}
