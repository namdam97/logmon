// Package http expose read API cho logpipeline BC: GET /logs tìm kiếm log.
package http

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/namdam97/logmon/backend/internal/logpipeline/domain"
	apperrors "github.com/namdam97/logmon/backend/internal/shared/errors"
	"github.com/namdam97/logmon/backend/internal/shared/httpx"
)

// LogSearchQueries là read-side use case handler phụ thuộc (ISP).
type LogSearchQueries interface {
	Search(ctx context.Context, in domain.SearchInput) (domain.SearchResult, error)
}

// LogHandler phục vụ truy vấn log. Mọi route yêu cầu đã đăng nhập.
type LogHandler struct {
	queries LogSearchQueries
}

// NewLogHandler tạo handler với query service được inject.
func NewLogHandler(queries LogSearchQueries) *LogHandler {
	return &LogHandler{queries: queries}
}

// Register gắn route GET /logs. authMW bảo vệ (người dùng đã đăng nhập).
func (h *LogHandler) Register(rg *gin.RouterGroup, authMW gin.HandlerFunc) {
	rg.GET("/logs", authMW, h.search)
}

func (h *LogHandler) search(c *gin.Context) {
	in, ok := parseSearchInput(c)
	if !ok {
		return // parseSearchInput đã ghi response 400
	}

	res, err := h.queries.Search(c.Request.Context(), in)
	if err != nil {
		if ve, isVE := apperrors.AsValidationError(err); isVE {
			httpx.Fail(c, http.StatusBadRequest, ve.Error())
			return
		}
		// Lỗi còn lại là từ backend lưu trữ (ES không tới được / lỗi truy vấn) →
		// 502; message generic, chi tiết đã nằm trong error wrap (không leak ra user).
		httpx.Fail(c, http.StatusBadGateway, "log store unavailable")
		return
	}

	httpx.OK(c, http.StatusOK, toSearchResponse(res))
}

// parseSearchInput đọc query params; trả ok=false (sau khi ghi 400) nếu sai định dạng.
func parseSearchInput(c *gin.Context) (domain.SearchInput, bool) {
	in := domain.SearchInput{
		Service:  c.Query("service"),
		Severity: c.Query("severity"),
		Query:    c.Query("q"),
		TraceID:  c.Query("trace_id"),
	}

	from, ok := parseTimeParam(c, "from")
	if !ok {
		return domain.SearchInput{}, false
	}
	in.From = from

	to, ok := parseTimeParam(c, "to")
	if !ok {
		return domain.SearchInput{}, false
	}
	in.To = to

	limit, ok := parseIntParam(c, "limit")
	if !ok {
		return domain.SearchInput{}, false
	}
	in.Limit = limit

	offset, ok := parseIntParam(c, "offset")
	if !ok {
		return domain.SearchInput{}, false
	}
	in.Offset = offset

	return in, true
}

func parseTimeParam(c *gin.Context, key string) (time.Time, bool) {
	raw := c.Query(key)
	if raw == "" {
		return time.Time{}, true
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid "+key+" timestamp (use RFC3339)")
		return time.Time{}, false
	}
	return t, true
}

func parseIntParam(c *gin.Context, key string) (int, bool) {
	raw := c.Query(key)
	if raw == "" {
		return 0, true
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid "+key+" (must be an integer)")
		return 0, false
	}
	return v, true
}

type logEntryResponse struct {
	Timestamp string `json:"timestamp"`
	Severity  string `json:"severity"`
	Body      string `json:"body"`
	Service   string `json:"service"`
	TraceID   string `json:"traceId"`
	SpanID    string `json:"spanId"`
}

type searchResponseBody struct {
	Total   int                `json:"total"`
	Entries []logEntryResponse `json:"entries"`
}

func toSearchResponse(res domain.SearchResult) searchResponseBody {
	entries := make([]logEntryResponse, 0, len(res.Entries))
	for _, e := range res.Entries {
		entries = append(entries, logEntryResponse{
			Timestamp: e.Timestamp.UTC().Format(time.RFC3339Nano),
			Severity:  e.Severity,
			Body:      e.Body,
			Service:   e.Service,
			TraceID:   e.TraceID,
			SpanID:    e.SpanID,
		})
	}
	return searchResponseBody{Total: res.Total, Entries: entries}
}
