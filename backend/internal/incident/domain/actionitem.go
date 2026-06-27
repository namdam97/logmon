package domain

import (
	"strings"
	"time"
)

// ActionItem là việc cần làm rút ra từ postmortem (doc_v2/06 §1.5): assignee +
// due date + trạng thái — nguồn dữ liệu thật cho "fewer repeat incidents". Bất biến.

const (
	_maxActionTitle = 500
	_maxAssignee    = 200
)

// ActionItemStatus là trạng thái một action item.
type ActionItemStatus struct {
	value string
}

// Ba trạng thái: open → in_progress → done.
var (
	ActionOpen       = ActionItemStatus{"open"}
	ActionInProgress = ActionItemStatus{"in_progress"}
	ActionDone       = ActionItemStatus{"done"}
)

var _validActionStatuses = map[string]ActionItemStatus{
	ActionOpen.value:       ActionOpen,
	ActionInProgress.value: ActionInProgress,
	ActionDone.value:       ActionDone,
}

// NewActionItemStatus validate và bọc một status string.
func NewActionItemStatus(raw string) (ActionItemStatus, error) {
	if s, ok := _validActionStatuses[raw]; ok {
		return s, nil
	}
	return ActionItemStatus{}, newValidationError("action_status", "must be one of open|in_progress|done")
}

// String trả về biểu diễn chuỗi của status.
func (s ActionItemStatus) String() string { return s.value }

// ActionItem là một việc theo dõi sau sự cố.
type ActionItem struct {
	id           string
	postmortemID PostmortemID
	title        string
	assignee     string
	dueDate      *time.Time
	status       ActionItemStatus
	createdAt    time.Time
	updatedAt    time.Time
	completedAt  *time.Time
}

// NewActionItemInput gom tham số tạo ActionItem.
type NewActionItemInput struct {
	ID           string
	PostmortemID PostmortemID
	Title        string
	Assignee     string
	DueDate      *time.Time
	Now          time.Time
}

// NewActionItem tạo action item mới ở trạng thái open.
func NewActionItem(in NewActionItemInput) (ActionItem, error) {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		return ActionItem{}, newValidationError("id", "must not be empty")
	}
	if in.PostmortemID.String() == "" {
		return ActionItem{}, newValidationError("postmortem_id", "must not be empty")
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return ActionItem{}, newValidationError("title", "must not be empty")
	}
	if len(title) > _maxActionTitle {
		return ActionItem{}, newValidationError("title", "too long")
	}
	if len(in.Assignee) > _maxAssignee {
		return ActionItem{}, newValidationError("assignee", "too long")
	}
	return ActionItem{
		id:           id,
		postmortemID: in.PostmortemID,
		title:        title,
		assignee:     strings.TrimSpace(in.Assignee),
		dueDate:      copyTime(in.DueDate),
		status:       ActionOpen,
		createdAt:    in.Now.UTC(),
		updatedAt:    in.Now.UTC(),
	}, nil
}

// UpdateStatus trả về bản sao với status mới. Done → ghi completedAt; rời khỏi
// done → xoá completedAt (reopen).
func (a ActionItem) UpdateStatus(status ActionItemStatus, now time.Time) ActionItem {
	cp := a
	cp.status = status
	cp.updatedAt = now.UTC()
	if status == ActionDone {
		at := now.UTC()
		cp.completedAt = &at
	} else {
		cp.completedAt = nil
	}
	return cp
}

// Accessors (read-only).

// ID trả về định danh action item.
func (a ActionItem) ID() string { return a.id }

// PostmortemID trả về postmortem chứa action item.
func (a ActionItem) PostmortemID() PostmortemID { return a.postmortemID }

// Title trả về tiêu đề việc cần làm.
func (a ActionItem) Title() string { return a.title }

// Assignee trả về người phụ trách.
func (a ActionItem) Assignee() string { return a.assignee }

// DueDate trả về hạn hoàn thành (nil nếu không đặt) — copy chống lộ con trỏ.
func (a ActionItem) DueDate() *time.Time { return copyTime(a.dueDate) }

// Status trả về trạng thái.
func (a ActionItem) Status() ActionItemStatus { return a.status }

// CreatedAt trả về thời điểm tạo (UTC).
func (a ActionItem) CreatedAt() time.Time { return a.createdAt }

// UpdatedAt trả về thời điểm cập nhật cuối (UTC).
func (a ActionItem) UpdatedAt() time.Time { return a.updatedAt }

// CompletedAt trả về thời điểm hoàn thành (nil nếu chưa) — copy chống lộ con trỏ.
func (a ActionItem) CompletedAt() *time.Time { return copyTime(a.completedAt) }

// ReconstructActionItem dựng lại ActionItem từ persistence.
func ReconstructActionItem(id, postmortemID, title, assignee, status string, dueDate, createdAt, updatedAt, completedAt *time.Time) ActionItem {
	st := ActionOpen
	if s, ok := _validActionStatuses[status]; ok {
		st = s
	}
	var created, updated time.Time
	if createdAt != nil {
		created = createdAt.UTC()
	}
	if updatedAt != nil {
		updated = updatedAt.UTC()
	}
	return ActionItem{
		id:           id,
		postmortemID: PostmortemID{value: postmortemID},
		title:        title,
		assignee:     assignee,
		dueDate:      copyTime(dueDate),
		status:       st,
		createdAt:    created,
		updatedAt:    updated,
		completedAt:  copyTime(completedAt),
	}
}
