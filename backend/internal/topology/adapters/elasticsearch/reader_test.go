package elasticsearch

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const _aggBody = `{"aggregations":{"sources":{"buckets":[
{"key":"gateway","targets":{"buckets":[
  {"key":"orders","doc_count":100,"errors":{"doc_count":10}},
  {"key":"users","doc_count":50,"errors":{"doc_count":0}}
]}},
{"key":"orders","targets":{"buckets":[
  {"key":"db","doc_count":80,"errors":{"doc_count":1}}
]}}
]}}}`

func TestDependenciesAggregatesEdges(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(_aggBody))
	}))
	defer srv.Close()

	edges, err := NewReader(srv.URL).Dependencies(context.Background(), "ws-1", time.Unix(0, 0).UTC())
	require.NoError(t, err)
	require.Len(t, edges, 3)

	require.Equal(t, "gateway", edges[0].Source)
	require.Equal(t, "orders", edges[0].Target)
	require.Equal(t, int64(100), edges[0].CallCount)
	require.Equal(t, int64(10), edges[0].ErrorCount)

	// workspace matcher được inject vào query DSL.
	require.Contains(t, string(gotBody), `"resource.attributes.workspace_id"`)
	require.Contains(t, string(gotBody), `ws-1`)
	require.Contains(t, string(gotBody), `"attributes.peer.service"`)
}

func TestDependenciesErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	_, err := NewReader(srv.URL).Dependencies(context.Background(), "ws-1", time.Unix(0, 0).UTC())
	require.Error(t, err)
	require.Contains(t, err.Error(), "unexpected status")
}

func TestDependenciesEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"aggregations":{"sources":{"buckets":[]}}}`))
	}))
	defer srv.Close()

	edges, err := NewReader(srv.URL).Dependencies(context.Background(), "ws-1", time.Unix(0, 0).UTC())
	require.NoError(t, err)
	require.Empty(t, edges)
}
