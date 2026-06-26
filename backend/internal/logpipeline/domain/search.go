package domain

import (
	"regexp"
	"strings"
	"time"

	apperrors "github.com/namdam97/logmon/backend/internal/shared/errors"
)

const (
	// DefaultLimit là số dòng trả về khi client không chỉ định limit.
	DefaultLimit = 100
	// MaxLimit chặn truy vấn quét quá lớn (bảo vệ ES + tránh unbounded query).
	MaxLimit = 1000

	_maxServiceLen = 200
	_maxQueryLen   = 500
)

// traceIDPattern: trace_id W3C 32 ký tự hex (khớp định dạng OTel/SpanContext).
var traceIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

// _validSeverities là tập severity_text hợp lệ (khớp level zerolog phát ra).
var _validSeverities = map[string]struct{}{
	"trace": {}, "debug": {}, "info": {}, "warn": {}, "error": {}, "fatal": {}, "panic": {},
}

// SearchInput là tham số thô (đã parse từ HTTP) để dựng SearchCriteria.
type SearchInput struct {
	Service  string
	Severity string
	Query    string
	TraceID  string
	From     time.Time
	To       time.Time
	Limit    int
	Offset   int
}

// SearchCriteria là value object đã validate cho một truy vấn log. Bất biến sau
// khi tạo — chỉ phơi accessor, adapter đọc qua đó để dựng query DSL.
type SearchCriteria struct {
	service  string
	severity string
	query    string
	traceID  string
	from     time.Time
	to       time.Time
	limit    int
	offset   int
}

// NewSearchCriteria validate + chuẩn hoá input. Trả về *ValidationError (shared)
// cho từng field sai để handler map sang HTTP 400.
func NewSearchCriteria(in SearchInput) (SearchCriteria, error) {
	limit := in.Limit
	switch {
	case limit == 0:
		limit = DefaultLimit
	case limit < 0:
		return SearchCriteria{}, apperrors.NewValidationError("limit", "must not be negative")
	case limit > MaxLimit:
		return SearchCriteria{}, apperrors.NewValidationError("limit", "exceeds maximum")
	}

	if in.Offset < 0 {
		return SearchCriteria{}, apperrors.NewValidationError("offset", "must not be negative")
	}

	if !in.From.IsZero() && !in.To.IsZero() && in.From.After(in.To) {
		return SearchCriteria{}, apperrors.NewValidationError("from", "must not be after to")
	}

	traceID := strings.TrimSpace(in.TraceID)
	if traceID != "" && !traceIDPattern.MatchString(traceID) {
		return SearchCriteria{}, apperrors.NewValidationError("trace_id", "must be 32 hex characters")
	}

	severity := strings.ToLower(strings.TrimSpace(in.Severity))
	if severity != "" {
		if _, ok := _validSeverities[severity]; !ok {
			return SearchCriteria{}, apperrors.NewValidationError("severity", "unknown severity level")
		}
	}

	service := strings.TrimSpace(in.Service)
	if len(service) > _maxServiceLen {
		return SearchCriteria{}, apperrors.NewValidationError("service", "too long")
	}

	query := strings.TrimSpace(in.Query)
	if len(query) > _maxQueryLen {
		return SearchCriteria{}, apperrors.NewValidationError("query", "too long")
	}

	return SearchCriteria{
		service:  service,
		severity: severity,
		query:    query,
		traceID:  traceID,
		from:     in.From,
		to:       in.To,
		limit:    limit,
		offset:   in.Offset,
	}, nil
}

// Service trả về filter service.name (rỗng = không lọc).
func (c SearchCriteria) Service() string { return c.service }

// Severity trả về filter severity_text đã lowercase (rỗng = không lọc).
func (c SearchCriteria) Severity() string { return c.severity }

// Query trả về chuỗi full-text match trên body (rỗng = không lọc).
func (c SearchCriteria) Query() string { return c.query }

// TraceID trả về filter trace_id (rỗng = không lọc).
func (c SearchCriteria) TraceID() string { return c.traceID }

// From trả về cận dưới khoảng thời gian; dùng HasFrom để biết có set hay không.
func (c SearchCriteria) From() time.Time { return c.from }

// To trả về cận trên khoảng thời gian; dùng HasTo để biết có set hay không.
func (c SearchCriteria) To() time.Time { return c.to }

// HasFrom báo có cận dưới thời gian.
func (c SearchCriteria) HasFrom() bool { return !c.from.IsZero() }

// HasTo báo có cận trên thời gian.
func (c SearchCriteria) HasTo() bool { return !c.to.IsZero() }

// Limit trả về số dòng tối đa mỗi trang.
func (c SearchCriteria) Limit() int { return c.limit }

// Offset trả về vị trí bắt đầu (phân trang).
func (c SearchCriteria) Offset() int { return c.offset }
