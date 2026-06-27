package domain

import (
	"strings"
	"time"

	apperrors "github.com/namdam97/logmon/backend/internal/shared/errors"
)

// DLQStatus là trạng thái xử lý một entry trong dead letter queue.
type DLQStatus int

// Enum bắt đầu từ 1.
const (
	// DLQPending: chưa xử lý, chờ người vận hành review.
	DLQPending DLQStatus = iota + 1
	// DLQRetried: đã retry (re-publish) sau review.
	DLQRetried
	// DLQDiscarded: bỏ qua, không retry.
	DLQDiscarded
)

// String trả về biểu diễn chuỗi khớp cột DB.
func (s DLQStatus) String() string {
	switch s {
	case DLQPending:
		return "pending"
	case DLQRetried:
		return "retried"
	case DLQDiscarded:
		return "discarded"
	default:
		return "unknown"
	}
}

// ParseDLQStatus chuyển chuỗi thành DLQStatus.
func ParseDLQStatus(raw string) (DLQStatus, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "pending":
		return DLQPending, nil
	case "retried":
		return DLQRetried, nil
	case "discarded":
		return DLQDiscarded, nil
	default:
		return 0, apperrors.NewValidationError("status", "must be pending|retried|discarded")
	}
}

// DLQEntry là một bản ghi log không nạp được vào ES (parse fail / ES reject).
// Bất biến — chuyển trạng thái trả bản sao mới.
type DLQEntry struct {
	id            int64
	workspaceID   string
	rawMessage    string
	errorReason   string
	sourceService string
	retryCount    int
	status        DLQStatus
	createdAt     time.Time
	retriedAt     *time.Time
}

// ReconstructDLQEntry dựng lại từ storage — KHÔNG validate lại.
func ReconstructDLQEntry(id int64, workspaceID, rawMessage, errorReason, sourceService string, retryCount int, status DLQStatus, createdAt time.Time, retriedAt *time.Time) DLQEntry {
	return DLQEntry{
		id: id, workspaceID: workspaceID, rawMessage: rawMessage, errorReason: errorReason,
		sourceService: sourceService, retryCount: retryCount, status: status,
		createdAt: createdAt, retriedAt: retriedAt,
	}
}

// ID trả về id (BIGINT identity).
func (e DLQEntry) ID() int64 { return e.id }

// WorkspaceID trả về workspace.
func (e DLQEntry) WorkspaceID() string { return e.workspaceID }

// RawMessage trả về message gốc không nạp được.
func (e DLQEntry) RawMessage() string { return e.rawMessage }

// ErrorReason trả về lý do thất bại.
func (e DLQEntry) ErrorReason() string { return e.errorReason }

// SourceService trả về service nguồn (có thể rỗng).
func (e DLQEntry) SourceService() string { return e.sourceService }

// RetryCount trả về số lần đã retry.
func (e DLQEntry) RetryCount() int { return e.retryCount }

// Status trả về trạng thái hiện tại.
func (e DLQEntry) Status() DLQStatus { return e.status }

// CreatedAt trả về thời điểm vào DLQ (UTC).
func (e DLQEntry) CreatedAt() time.Time { return e.createdAt }

// RetriedAt trả về thời điểm retry gần nhất (nil nếu chưa).
func (e DLQEntry) RetriedAt() *time.Time { return e.retriedAt }

// MarkRetried trả bản sao đã đánh dấu retried (tăng retryCount, set retriedAt).
// Chỉ entry pending mới retry được.
func (e DLQEntry) MarkRetried(now time.Time) (DLQEntry, error) {
	if e.status != DLQPending {
		return DLQEntry{}, ErrDLQNotRetryable
	}
	cp := e
	cp.status = DLQRetried
	cp.retryCount = e.retryCount + 1
	t := now
	cp.retriedAt = &t
	return cp, nil
}
