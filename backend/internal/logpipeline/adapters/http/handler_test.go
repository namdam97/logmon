package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	loghttp "github.com/namdam97/logmon/backend/internal/logpipeline/adapters/http"
	"github.com/namdam97/logmon/backend/internal/logpipeline/domain"
	apperrors "github.com/namdam97/logmon/backend/internal/shared/errors"
)

func init() { gin.SetMode(gin.TestMode) }

type stubQueries struct {
	got    domain.SearchInput
	result domain.SearchResult
	err    error
}

func (s *stubQueries) Search(_ context.Context, in domain.SearchInput) (domain.SearchResult, error) {
	s.got = in
	return s.result, s.err
}

func newRouter(q loghttp.LogSearchQueries) *gin.Engine {
	r := gin.New()
	h := loghttp.NewLogHandler(q)
	// authMW no-op cho test.
	h.Register(r.Group("/api/v1"), func(c *gin.Context) { c.Next() })
	return r
}

func doGet(t *testing.T, r *gin.Engine, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestSearchParsesQueryParams(t *testing.T) {
	stub := &stubQueries{result: domain.SearchResult{
		Entries: []domain.LogEntry{{
			Timestamp: time.Date(2026, 6, 27, 10, 0, 0, 0, time.UTC),
			Severity:  "error",
			Body:      "boom",
			Service:   "userservice",
			TraceID:   "0af7651916cd43dd8448eb211c80319c",
		}},
		Total: 1,
	}}
	r := newRouter(stub)

	w := doGet(t, r, "/api/v1/logs?service=userservice&severity=error&q=boom&limit=25&offset=5&from=2026-06-27T09:00:00Z&to=2026-06-27T11:00:00Z")

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "userservice", stub.got.Service)
	require.Equal(t, "error", stub.got.Severity)
	require.Equal(t, "boom", stub.got.Query)
	require.Equal(t, 25, stub.got.Limit)
	require.Equal(t, 5, stub.got.Offset)
	require.Equal(t, time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC), stub.got.From)
	require.Equal(t, time.Date(2026, 6, 27, 11, 0, 0, 0, time.UTC), stub.got.To)

	var env struct {
		Success bool `json:"success"`
		Data    struct {
			Total   int `json:"total"`
			Entries []struct {
				Body    string `json:"body"`
				TraceID string `json:"traceId"`
			} `json:"entries"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.True(t, env.Success)
	require.Equal(t, 1, env.Data.Total)
	require.Len(t, env.Data.Entries, 1)
	require.Equal(t, "boom", env.Data.Entries[0].Body)
}

func TestSearchRejectsBadTime(t *testing.T) {
	r := newRouter(&stubQueries{})

	w := doGet(t, r, "/api/v1/logs?from=not-a-time")

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSearchRejectsBadLimit(t *testing.T) {
	r := newRouter(&stubQueries{})

	w := doGet(t, r, "/api/v1/logs?limit=abc")

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSearchMapsValidationErrorTo400(t *testing.T) {
	// Use case trả ValidationError (vd limit vượt max) → handler map 400.
	r := newRouter(&stubQueries{err: apperrors.NewValidationError("limit", "exceeds maximum")})

	w := doGet(t, r, "/api/v1/logs")

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSearchMapsUpstreamErrorTo502(t *testing.T) {
	r := newRouter(&stubQueries{err: context.DeadlineExceeded})

	w := doGet(t, r, "/api/v1/logs")

	require.Equal(t, http.StatusBadGateway, w.Code)
}
