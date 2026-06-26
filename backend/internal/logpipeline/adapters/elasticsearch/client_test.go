package elasticsearch_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/logpipeline/adapters/elasticsearch"
	"github.com/namdam97/logmon/backend/internal/logpipeline/domain"
)

// esResponse là phần ES trả về _search; test dựng response tối thiểu OTel-native.
const esResponse = `{
  "hits": {
    "total": {"value": 2, "relation": "eq"},
    "hits": [
      {"_source": {
        "@timestamp": "2026-06-27T10:00:00Z",
        "severity_text": "error",
        "body": {"text": "db timeout"},
        "trace_id": "0af7651916cd43dd8448eb211c80319c",
        "span_id": "b7ad6b7169203331",
        "resource": {"attributes": {"service.name": "userservice"}}
      }}
    ]
  }
}`

func newCriteria(t *testing.T, in domain.SearchInput) domain.SearchCriteria {
	t.Helper()
	c, err := domain.NewSearchCriteria(in)
	require.NoError(t, err)
	return c
}

func TestSearchBuildsQueryAndParsesHits(t *testing.T) {
	from := time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC)
	to := from.Add(2 * time.Hour)

	var gotPath, gotAuthUser, gotAuthPass string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuthUser, gotAuthPass, _ = r.BasicAuth()
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, esResponse)
	}))
	defer srv.Close()

	client := elasticsearch.NewClient(srv.URL, "elastic", "secret")
	crit := newCriteria(t, domain.SearchInput{
		Service:  "userservice",
		Severity: "error",
		Query:    "timeout",
		TraceID:  "0af7651916cd43dd8448eb211c80319c",
		From:     from,
		To:       to,
		Limit:    50,
		Offset:   10,
	})

	// Act
	res, err := client.Search(context.Background(), crit)

	// Assert: request đúng index data stream + basic auth.
	require.NoError(t, err)
	require.Equal(t, "/logs-*/_search", gotPath)
	require.Equal(t, "elastic", gotAuthUser)
	require.Equal(t, "secret", gotAuthPass)

	// Body mang size/from + bool filter.
	require.EqualValues(t, 50, gotBody["size"])
	require.EqualValues(t, 10, gotBody["from"])
	require.NotNil(t, gotBody["query"])

	// Parse hits.
	require.Equal(t, 2, res.Total)
	require.Len(t, res.Entries, 1)
	e := res.Entries[0]
	require.Equal(t, "error", e.Severity)
	require.Equal(t, "db timeout", e.Body)
	require.Equal(t, "userservice", e.Service)
	require.Equal(t, "0af7651916cd43dd8448eb211c80319c", e.TraceID)
	require.Equal(t, "b7ad6b7169203331", e.SpanID)
	require.Equal(t, time.Date(2026, 6, 27, 10, 0, 0, 0, time.UTC), e.Timestamp)
}

func TestSearchNoFiltersStillQueries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"hits":{"total":{"value":0},"hits":[]}}`)
	}))
	defer srv.Close()

	client := elasticsearch.NewClient(srv.URL, "elastic", "secret")
	res, err := client.Search(context.Background(), newCriteria(t, domain.SearchInput{}))

	require.NoError(t, err)
	require.Equal(t, 0, res.Total)
	require.Empty(t, res.Entries)
}

func TestSearchErrorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := elasticsearch.NewClient(srv.URL, "elastic", "secret")
	_, err := client.Search(context.Background(), newCriteria(t, domain.SearchInput{}))

	require.Error(t, err)
}
