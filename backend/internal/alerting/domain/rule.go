package domain

import (
	"strings"
	"time"
)

const (
	maxNameLength    = 100
	maxServiceLength = 100
)

// Annotation key bắt buộc trên mọi rule (doc_v2/05 §1) — không có thì không activate.
const (
	AnnotationSummary    = "summary"
	AnnotationRunbookURL = "runbook_url"
)

// SyncStatus phản ánh trạng thái đồng bộ rule sang Prometheus (rule sync pipeline).
type SyncStatus string

// Các trạng thái sync của rule sang Prometheus.
const (
	SyncPending SyncStatus = "pending"
	SyncSynced  SyncStatus = "synced"
	SyncError   SyncStatus = "error"
)

// RuleID là value object định danh rule (UUID dạng chuỗi).
type RuleID struct {
	value string
}

// NewRuleID validate và bọc định danh rule không rỗng.
func NewRuleID(raw string) (RuleID, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return RuleID{}, newValidationError("id", "must not be empty")
	}
	return RuleID{value: v}, nil
}

// String trả về biểu diễn chuỗi của RuleID.
func (id RuleID) String() string { return id.value }

// AlertRule là aggregate root của alerting BC. Field không export để giữ bất biến
// — chỉ tạo qua NewAlertRule, chuyển trạng thái qua method trả bản copy mới.
type AlertRule struct {
	id          RuleID
	workspaceID string
	name        string
	expression  string
	forDuration time.Duration
	severity    Severity
	service     string
	labels      map[string]string
	annotations map[string]string
	enabled     bool
	syncStatus  SyncStatus
	syncError   string
	createdAt   time.Time
	updatedAt   time.Time
}

// NewAlertRuleInput gom tham số tạo rule (đã đi qua value object Severity/RuleID).
type NewAlertRuleInput struct {
	ID          RuleID
	WorkspaceID string
	Name        string
	Expression  string
	Service     string
	ForDuration time.Duration
	Severity    Severity
	Labels      map[string]string
	Annotations map[string]string
	CreatedAt   time.Time
}

// NewAlertRule tạo rule mới đã validate đầy đủ invariant. Expression CHỈ kiểm
// non-empty ở domain; validate cú pháp PromQL do tầng app (ports.RuleValidator).
func NewAlertRule(in NewAlertRuleInput) (AlertRule, error) {
	name := strings.TrimSpace(in.Name)
	switch {
	case name == "":
		return AlertRule{}, newValidationError("name", "must not be empty")
	case len(name) > maxNameLength:
		return AlertRule{}, newValidationError("name", "exceeds maximum length")
	}
	if strings.TrimSpace(in.WorkspaceID) == "" {
		return AlertRule{}, newValidationError("workspaceId", "must not be empty")
	}
	if strings.TrimSpace(in.Expression) == "" {
		return AlertRule{}, newValidationError("expression", "must not be empty")
	}
	service := strings.TrimSpace(in.Service)
	switch {
	case service == "":
		return AlertRule{}, newValidationError("service", "must not be empty")
	case len(service) > maxServiceLength:
		return AlertRule{}, newValidationError("service", "exceeds maximum length")
	}
	if strings.TrimSpace(in.Annotations[AnnotationSummary]) == "" {
		return AlertRule{}, newValidationError("annotations.summary", "is required")
	}
	if strings.TrimSpace(in.Annotations[AnnotationRunbookURL]) == "" {
		return AlertRule{}, newValidationError("annotations.runbook_url", "is required")
	}
	if minDur := in.Severity.MinForDuration(); in.ForDuration < minDur {
		return AlertRule{}, newValidationError("forDuration", "below minimum for severity")
	}
	if in.CreatedAt.IsZero() {
		return AlertRule{}, newValidationError("createdAt", "must be set")
	}

	return AlertRule{
		id:          in.ID,
		workspaceID: in.WorkspaceID,
		name:        name,
		expression:  strings.TrimSpace(in.Expression),
		forDuration: in.ForDuration,
		severity:    in.Severity,
		service:     service,
		labels:      copyMap(in.Labels),
		annotations: copyMap(in.Annotations),
		enabled:     true,
		syncStatus:  SyncPending,
		createdAt:   in.CreatedAt,
		updatedAt:   in.CreatedAt,
	}, nil
}

// Enabled trả về bản copy đã bật rule (sync về pending để render lại).
func (r AlertRule) Enabled(now time.Time) AlertRule {
	c := r.clone()
	c.enabled = true
	c.syncStatus = SyncPending
	c.updatedAt = now
	return c
}

// Disabled trả về bản copy đã tắt rule.
func (r AlertRule) Disabled(now time.Time) AlertRule {
	c := r.clone()
	c.enabled = false
	c.syncStatus = SyncPending
	c.updatedAt = now
	return c
}

// MarkSynced đánh dấu rule đã sync thành công sang Prometheus.
func (r AlertRule) MarkSynced(now time.Time) AlertRule {
	c := r.clone()
	c.syncStatus = SyncSynced
	c.syncError = ""
	c.updatedAt = now
	return c
}

// MarkSyncError đánh dấu rule sync lỗi kèm thông điệp.
func (r AlertRule) MarkSyncError(message string, now time.Time) AlertRule {
	c := r.clone()
	c.syncStatus = SyncError
	c.syncError = message
	c.updatedAt = now
	return c
}

// ID trả về định danh rule.
func (r AlertRule) ID() RuleID { return r.id }

// WorkspaceID trả về workspace sở hữu rule.
func (r AlertRule) WorkspaceID() string { return r.workspaceID }

// Name trả về tên rule (duy nhất trong workspace).
func (r AlertRule) Name() string { return r.name }

// Expression trả về biểu thức PromQL.
func (r AlertRule) Expression() string { return r.expression }

// ForDuration trả về thời lượng `for` của rule.
func (r AlertRule) ForDuration() time.Duration { return r.forDuration }

// Severity trả về mức độ của rule.
func (r AlertRule) Severity() Severity { return r.severity }

// Service trả về service mà rule áp dụng.
func (r AlertRule) Service() string { return r.service }

// Labels trả về bản copy labels của rule.
func (r AlertRule) Labels() map[string]string { return copyMap(r.labels) }

// Annotations trả về bản copy annotations của rule.
func (r AlertRule) Annotations() map[string]string { return copyMap(r.annotations) }

// IsEnabled cho biết rule có đang bật không.
func (r AlertRule) IsEnabled() bool { return r.enabled }

// SyncStatus trả về trạng thái sync sang Prometheus.
func (r AlertRule) SyncStatus() SyncStatus { return r.syncStatus }

// SyncError trả về thông điệp lỗi sync gần nhất (rỗng nếu không lỗi).
func (r AlertRule) SyncError() string { return r.syncError }

// CreatedAt trả về thời điểm tạo rule.
func (r AlertRule) CreatedAt() time.Time { return r.createdAt }

// UpdatedAt trả về thời điểm cập nhật rule gần nhất.
func (r AlertRule) UpdatedAt() time.Time { return r.updatedAt }

func (r AlertRule) clone() AlertRule {
	c := r
	c.labels = copyMap(r.labels)
	c.annotations = copyMap(r.annotations)
	return c
}

func copyMap(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
