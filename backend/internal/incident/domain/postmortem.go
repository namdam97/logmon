package domain

import (
	"strings"
	"time"
)

// Postmortem blameless (doc_v2/06 §1.5). Bắt buộc cho SEV1/SEV2. Cấu trúc: root
// cause · impact (số liệu từ chính LogMon: thời lượng/error count/budget tiêu thụ)
// · timeline summary · lessons learned · action items (aggregate con). Là aggregate
// riêng liên kết incident 1-1; bất biến (method trả copy). KHÔNG có field "đổ lỗi"
// — blameless theo thiết kế.

const (
	_maxRootCause     = 5000
	_maxLessons       = 5000
	_maxTimelineSum   = 5000
	_maxImpactSummary = 2000
)

// PostmortemID là value object định danh postmortem (UUID dạng chuỗi).
type PostmortemID struct {
	value string
}

// NewPostmortemID validate và bọc định danh postmortem không rỗng.
func NewPostmortemID(raw string) (PostmortemID, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return PostmortemID{}, newValidationError("postmortem_id", "must not be empty")
	}
	return PostmortemID{value: v}, nil
}

// String trả về biểu diễn chuỗi của PostmortemID.
func (id PostmortemID) String() string { return id.value }

// PostmortemStatus là trạng thái soạn thảo postmortem.
type PostmortemStatus struct {
	value string
}

// Hai trạng thái: draft (đang soạn) và published (đã chốt).
var (
	PostmortemDraft     = PostmortemStatus{"draft"}
	PostmortemPublished = PostmortemStatus{"published"}
)

var _validPostmortemStatuses = map[string]PostmortemStatus{
	PostmortemDraft.value:     PostmortemDraft,
	PostmortemPublished.value: PostmortemPublished,
}

// NewPostmortemStatus validate và bọc status string (dùng khi hydrate từ DB).
func NewPostmortemStatus(raw string) (PostmortemStatus, error) {
	if s, ok := _validPostmortemStatuses[raw]; ok {
		return s, nil
	}
	return PostmortemStatus{}, newValidationError("postmortem_status", "unknown status")
}

// String trả về biểu diễn chuỗi của status.
func (s PostmortemStatus) String() string { return s.value }

// Impact gom số liệu tác động đo từ chính LogMon (doc_v2/06 §1.5).
type Impact struct {
	DurationSeconds       int64   // thời lượng sự cố (suy từ incident MTTR)
	ErrorCount            int64   // số lỗi trong cửa sổ sự cố
	BudgetConsumedPercent float64 // % error budget tiêu thụ
	Summary               string  // mô tả tác động (user nhập)
}

// validate kiểm tra Impact hợp lệ.
func (i Impact) validate() error {
	if i.DurationSeconds < 0 {
		return newValidationError("impact.duration", "must not be negative")
	}
	if i.ErrorCount < 0 {
		return newValidationError("impact.error_count", "must not be negative")
	}
	if len(i.Summary) > _maxImpactSummary {
		return newValidationError("impact.summary", "too long")
	}
	return nil
}

// Postmortem là aggregate postmortem của một incident.
type Postmortem struct {
	id              PostmortemID
	incidentID      IncidentID
	workspaceID     string
	status          PostmortemStatus
	rootCause       string
	impact          Impact
	timelineSummary string
	lessonsLearned  string
	createdAt       time.Time
	updatedAt       time.Time
	publishedAt     *time.Time
}

// NewPostmortemInput gom tham số tạo Postmortem (draft).
type NewPostmortemInput struct {
	ID              string
	IncidentID      IncidentID
	WorkspaceID     string
	RootCause       string
	Impact          Impact
	TimelineSummary string
	LessonsLearned  string
	Now             time.Time
}

// NewPostmortem tạo postmortem mới ở trạng thái draft. Cho phép field nội dung
// rỗng khi draft — ràng buộc đầy đủ áp ở Publish.
func NewPostmortem(in NewPostmortemInput) (Postmortem, error) {
	id, err := NewPostmortemID(in.ID)
	if err != nil {
		return Postmortem{}, err
	}
	if in.IncidentID.String() == "" {
		return Postmortem{}, newValidationError("incident_id", "must not be empty")
	}
	if strings.TrimSpace(in.WorkspaceID) == "" {
		return Postmortem{}, newValidationError("workspace_id", "must not be empty")
	}
	if err := validateContentLengths(in.RootCause, in.TimelineSummary, in.LessonsLearned); err != nil {
		return Postmortem{}, err
	}
	if err := in.Impact.validate(); err != nil {
		return Postmortem{}, err
	}
	return Postmortem{
		id:              id,
		incidentID:      in.IncidentID,
		workspaceID:     in.WorkspaceID,
		status:          PostmortemDraft,
		rootCause:       in.RootCause,
		impact:          in.Impact,
		timelineSummary: in.TimelineSummary,
		lessonsLearned:  in.LessonsLearned,
		createdAt:       in.Now.UTC(),
		updatedAt:       in.Now.UTC(),
	}, nil
}

func validateContentLengths(rootCause, timelineSummary, lessons string) error {
	if len(rootCause) > _maxRootCause {
		return newValidationError("root_cause", "too long")
	}
	if len(timelineSummary) > _maxTimelineSum {
		return newValidationError("timeline_summary", "too long")
	}
	if len(lessons) > _maxLessons {
		return newValidationError("lessons_learned", "too long")
	}
	return nil
}

// UpdateContentInput gom field cập nhật nội dung postmortem.
type UpdateContentInput struct {
	RootCause       string
	Impact          Impact
	TimelineSummary string
	LessonsLearned  string
	Now             time.Time
}

// UpdateContent trả về bản sao với nội dung mới. Không cho sửa khi đã published
// (ErrPostmortemPublished) — chốt rồi thì bất biến.
func (p Postmortem) UpdateContent(in UpdateContentInput) (Postmortem, error) {
	if p.status == PostmortemPublished {
		return Postmortem{}, ErrPostmortemPublished
	}
	if err := validateContentLengths(in.RootCause, in.TimelineSummary, in.LessonsLearned); err != nil {
		return Postmortem{}, err
	}
	if err := in.Impact.validate(); err != nil {
		return Postmortem{}, err
	}
	cp := p
	cp.rootCause = in.RootCause
	cp.impact = in.Impact
	cp.timelineSummary = in.TimelineSummary
	cp.lessonsLearned = in.LessonsLearned
	cp.updatedAt = in.Now.UTC()
	return cp, nil
}

// Publish chuyển draft→published. Yêu cầu root cause + lessons learned non-empty
// (postmortem hữu ích mới chốt được). Đã published → ErrPostmortemPublished.
func (p Postmortem) Publish(now time.Time) (Postmortem, error) {
	if p.status == PostmortemPublished {
		return Postmortem{}, ErrPostmortemPublished
	}
	if strings.TrimSpace(p.rootCause) == "" {
		return Postmortem{}, newValidationError("root_cause", "required before publishing")
	}
	if strings.TrimSpace(p.lessonsLearned) == "" {
		return Postmortem{}, newValidationError("lessons_learned", "required before publishing")
	}
	cp := p
	cp.status = PostmortemPublished
	at := now.UTC()
	cp.publishedAt = &at
	cp.updatedAt = at
	return cp, nil
}

// Accessors (read-only).

// ID trả về định danh postmortem.
func (p Postmortem) ID() PostmortemID { return p.id }

// IncidentID trả về incident liên kết.
func (p Postmortem) IncidentID() IncidentID { return p.incidentID }

// WorkspaceID trả về workspace sở hữu.
func (p Postmortem) WorkspaceID() string { return p.workspaceID }

// Status trả về trạng thái soạn thảo.
func (p Postmortem) Status() PostmortemStatus { return p.status }

// RootCause trả về nguyên nhân gốc.
func (p Postmortem) RootCause() string { return p.rootCause }

// Impact trả về số liệu tác động.
func (p Postmortem) Impact() Impact { return p.impact }

// TimelineSummary trả về tóm tắt dòng thời gian.
func (p Postmortem) TimelineSummary() string { return p.timelineSummary }

// LessonsLearned trả về bài học rút ra.
func (p Postmortem) LessonsLearned() string { return p.lessonsLearned }

// CreatedAt trả về thời điểm tạo (UTC).
func (p Postmortem) CreatedAt() time.Time { return p.createdAt }

// UpdatedAt trả về thời điểm cập nhật cuối (UTC).
func (p Postmortem) UpdatedAt() time.Time { return p.updatedAt }

// PublishedAt trả về thời điểm chốt (nil nếu chưa published) — copy chống lộ con trỏ.
func (p Postmortem) PublishedAt() *time.Time { return copyTime(p.publishedAt) }

// ReconstructPostmortemInput hydrate Postmortem từ DB (đã validate khi ghi).
type ReconstructPostmortemInput struct {
	ID              string
	IncidentID      string
	WorkspaceID     string
	Status          string
	RootCause       string
	Impact          Impact
	TimelineSummary string
	LessonsLearned  string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	PublishedAt     *time.Time
}

// ReconstructPostmortem dựng lại Postmortem từ persistence.
func ReconstructPostmortem(in ReconstructPostmortemInput) Postmortem {
	status := PostmortemDraft
	if s, ok := _validPostmortemStatuses[in.Status]; ok {
		status = s
	}
	return Postmortem{
		id:              PostmortemID{value: in.ID},
		incidentID:      IncidentID{value: in.IncidentID},
		workspaceID:     in.WorkspaceID,
		status:          status,
		rootCause:       in.RootCause,
		impact:          in.Impact,
		timelineSummary: in.TimelineSummary,
		lessonsLearned:  in.LessonsLearned,
		createdAt:       in.CreatedAt.UTC(),
		updatedAt:       in.UpdatedAt.UTC(),
		publishedAt:     copyTime(in.PublishedAt),
	}
}
