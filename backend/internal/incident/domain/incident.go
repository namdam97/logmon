package domain

import (
	"regexp"
	"strings"
	"time"
)

const (
	maxTitleLength       = 200
	maxDescriptionLength = 2000
	maxServiceLength     = 100
	maxAssigneeLength    = 200
	maxSourceRefLength   = 200
)

// _safeServicePattern giới hạn ký tự service (dùng làm Prometheus label —
// cardinality + an toàn): chữ, số, khoảng trắng, '_' '-' '.'. Khớp với SLO
// service pattern để auto-create từ BudgetExhausted không bị reject.
var _safeServicePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9 _.\-]*$`)

// Incident là aggregate root của incident BC. Field không export để giữ bất biến
// — chỉ tạo qua NewIncident, chuyển trạng thái qua method trả bản copy mới
// (immutability). State machine 7 trạng thái thực thi trong các transition method.
type Incident struct {
	id          IncidentID
	workspaceID string
	title       string
	description string
	service     string
	severity    Severity // zero = chưa phân loại (set khi Triage)
	status      Status
	source      Source
	sourceRef   string // alert fingerprint / slo id — dedup auto-create
	assignee    string
	createdAt   time.Time
	updatedAt   time.Time
	assignedAt  *time.Time // lần assign ĐẦU TIÊN → MTTA
	resolvedAt  *time.Time // → MTTR
	closedAt    *time.Time
}

// NewIncidentInput gom tham số tạo incident.
type NewIncidentInput struct {
	ID          IncidentID
	WorkspaceID string
	Title       string
	Description string
	Service     string
	Severity    Severity // optional — auto-create từ budget set sẵn SEV2
	Source      Source
	SourceRef   string
	CreatedAt   time.Time
}

// NewIncident tạo incident mới ở trạng thái Open, đã validate invariant.
func NewIncident(in NewIncidentInput) (Incident, error) {
	title, err := validateTitle(in.Title)
	if err != nil {
		return Incident{}, err
	}
	if strings.TrimSpace(in.WorkspaceID) == "" {
		return Incident{}, newValidationError("workspaceId", "must not be empty")
	}
	service, err := validateService(in.Service)
	if err != nil {
		return Incident{}, err
	}
	description, err := validateDescription(in.Description)
	if err != nil {
		return Incident{}, err
	}
	if (in.Source == Source{}) {
		return Incident{}, newValidationError("source", "must be set")
	}
	if len(in.SourceRef) > maxSourceRefLength {
		return Incident{}, newValidationError("sourceRef", "exceeds maximum length")
	}
	if in.CreatedAt.IsZero() {
		return Incident{}, newValidationError("createdAt", "must be set")
	}

	return Incident{
		id:          in.ID,
		workspaceID: in.WorkspaceID,
		title:       title,
		description: description,
		service:     service,
		severity:    in.Severity,
		status:      StatusOpen,
		source:      in.Source,
		sourceRef:   strings.TrimSpace(in.SourceRef),
		createdAt:   in.CreatedAt,
		updatedAt:   in.CreatedAt,
	}, nil
}

func validateTitle(raw string) (string, error) {
	title := strings.TrimSpace(raw)
	switch {
	case title == "":
		return "", newValidationError("title", "must not be empty")
	case len(title) > maxTitleLength:
		return "", newValidationError("title", "exceeds maximum length")
	}
	return title, nil
}

func validateService(raw string) (string, error) {
	service := strings.TrimSpace(raw)
	switch {
	case service == "":
		return "", newValidationError("service", "must not be empty")
	case len(service) > maxServiceLength:
		return "", newValidationError("service", "exceeds maximum length")
	case !_safeServicePattern.MatchString(service):
		return "", newValidationError("service", "contains invalid characters")
	}
	return service, nil
}

func validateDescription(raw string) (string, error) {
	d := strings.TrimSpace(raw)
	if len(d) > maxDescriptionLength {
		return "", newValidationError("description", "exceeds maximum length")
	}
	return d, nil
}

// Triage chuyển Open → Triaged: gán severity + cập nhật impact (description).
// Trả ErrInvalidTransition nếu không ở Open.
func (i Incident) Triage(severity Severity, description string, now time.Time) (Incident, error) {
	if i.status != StatusOpen {
		return Incident{}, wrapTransition("triage", i.status)
	}
	if severity.IsZero() {
		return Incident{}, newValidationError("severity", "must be set when triaging")
	}
	desc, err := validateDescription(description)
	if err != nil {
		return Incident{}, err
	}
	c := i
	c.severity = severity
	if desc != "" {
		c.description = desc
	}
	c.status = StatusTriaged
	c.updatedAt = now
	return c, nil
}

// Assign chuyển Triaged → Assigned (assign on-call) hoặc Mitigating → Assigned
// (re-assign / escalation). Ghi assignedAt lần đầu để tính MTTA.
func (i Incident) Assign(assignee string, now time.Time) (Incident, error) {
	if i.status != StatusTriaged && i.status != StatusMitigating {
		return Incident{}, wrapTransition("assign", i.status)
	}
	a := strings.TrimSpace(assignee)
	switch {
	case a == "":
		return Incident{}, newValidationError("assignee", "must not be empty")
	case len(a) > maxAssigneeLength:
		return Incident{}, newValidationError("assignee", "exceeds maximum length")
	}
	c := i
	c.assignee = a
	c.status = StatusAssigned
	if c.assignedAt == nil {
		t := now
		c.assignedAt = &t
	}
	c.updatedAt = now
	return c, nil
}

// StartMitigation chuyển Assigned → Mitigating (engineer bắt đầu xử lý).
func (i Incident) StartMitigation(now time.Time) (Incident, error) {
	if i.status != StatusAssigned {
		return Incident{}, wrapTransition("start mitigation", i.status)
	}
	c := i
	c.status = StatusMitigating
	c.updatedAt = now
	return c, nil
}

// Resolve chuyển Open → Resolved (false alarm / auto-resolve) hoặc
// Mitigating → Resolved (fix deployed). Ghi resolvedAt để tính MTTR.
func (i Incident) Resolve(now time.Time) (Incident, error) {
	if i.status != StatusOpen && i.status != StatusMitigating {
		return Incident{}, wrapTransition("resolve", i.status)
	}
	c := i
	c.status = StatusResolved
	t := now
	c.resolvedAt = &t
	c.updatedAt = now
	return c, nil
}

// RequirePostmortem chuyển Resolved → PostmortemPending — chỉ SEV1/SEV2 (bắt
// buộc postmortem). Thường gọi tự động sau 24h (doc_v2/06 §1.1).
func (i Incident) RequirePostmortem(now time.Time) (Incident, error) {
	if i.status != StatusResolved {
		return Incident{}, wrapTransition("require postmortem", i.status)
	}
	if !i.severity.RequiresPostmortem() {
		return Incident{}, newValidationError("severity", "postmortem only required for SEV1/SEV2")
	}
	c := i
	c.status = StatusPostmortemPending
	c.updatedAt = now
	return c, nil
}

// Close chuyển Resolved → Closed (SEV3/SEV4, postmortem optional) hoặc
// PostmortemPending → Closed (postmortem hoàn thành). SEV1/SEV2 đang Resolved
// PHẢI qua PostmortemPending trước (ErrInvalidTransition).
func (i Incident) Close(now time.Time) (Incident, error) {
	switch i.status {
	case StatusPostmortemPending:
		// luôn cho phép — postmortem đã xong.
	case StatusResolved:
		if i.severity.RequiresPostmortem() {
			return Incident{}, wrapTransition("close", i.status)
		}
	default:
		return Incident{}, wrapTransition("close", i.status)
	}
	c := i
	c.status = StatusClosed
	t := now
	c.closedAt = &t
	c.updatedAt = now
	return c, nil
}

// MTTA trả về thời gian từ tạo → assign lần đầu; ok=false nếu chưa assign.
func (i Incident) MTTA() (time.Duration, bool) {
	if i.assignedAt == nil {
		return 0, false
	}
	return i.assignedAt.Sub(i.createdAt), true
}

// MTTR trả về thời gian từ tạo → resolved; ok=false nếu chưa resolved.
func (i Incident) MTTR() (time.Duration, bool) {
	if i.resolvedAt == nil {
		return 0, false
	}
	return i.resolvedAt.Sub(i.createdAt), true
}

// ReconstructInput dựng lại Incident từ dữ liệu DB đã hợp lệ.
type ReconstructInput struct {
	ID          IncidentID
	WorkspaceID string
	Title       string
	Description string
	Service     string
	Severity    Severity
	Status      Status
	Source      Source
	SourceRef   string
	Assignee    string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	AssignedAt  *time.Time
	ResolvedAt  *time.Time
	ClosedAt    *time.Time
}

// Reconstruct hydrate Incident từ persistence. Tin dữ liệu DB.
func Reconstruct(in ReconstructInput) Incident {
	return Incident{
		id:          in.ID,
		workspaceID: in.WorkspaceID,
		title:       in.Title,
		description: in.Description,
		service:     in.Service,
		severity:    in.Severity,
		status:      in.Status,
		source:      in.Source,
		sourceRef:   in.SourceRef,
		assignee:    in.Assignee,
		createdAt:   in.CreatedAt,
		updatedAt:   in.UpdatedAt,
		assignedAt:  in.AssignedAt,
		resolvedAt:  in.ResolvedAt,
		closedAt:    in.ClosedAt,
	}
}

func wrapTransition(action string, from Status) error {
	return &transitionError{action: action, from: from}
}

// transitionError bọc ErrInvalidTransition kèm ngữ cảnh (action + from status)
// nhưng vẫn match được qua errors.Is(err, ErrInvalidTransition).
type transitionError struct {
	action string
	from   Status
}

func (e *transitionError) Error() string {
	return "incident: cannot " + e.action + " from status " + e.from.String()
}

func (e *transitionError) Unwrap() error { return ErrInvalidTransition }

// ---- Accessors (read-only, trả copy con trỏ thời gian để giữ bất biến) ----

// ID trả về định danh incident.
func (i Incident) ID() IncidentID { return i.id }

// WorkspaceID trả về workspace sở hữu incident.
func (i Incident) WorkspaceID() string { return i.workspaceID }

// Title trả về tiêu đề incident.
func (i Incident) Title() string { return i.title }

// Description trả về mô tả/impact.
func (i Incident) Description() string { return i.description }

// Service trả về service bị ảnh hưởng.
func (i Incident) Service() string { return i.service }

// Severity trả về mức severity (zero nếu chưa phân loại).
func (i Incident) Severity() Severity { return i.severity }

// Status trả về trạng thái hiện tại.
func (i Incident) Status() Status { return i.status }

// Source trả về nguồn tạo incident.
func (i Incident) Source() Source { return i.source }

// SourceRef trả về tham chiếu nguồn (alert fingerprint / slo id).
func (i Incident) SourceRef() string { return i.sourceRef }

// Assignee trả về người được giao (rỗng nếu chưa assign).
func (i Incident) Assignee() string { return i.assignee }

// CreatedAt trả về thời điểm tạo.
func (i Incident) CreatedAt() time.Time { return i.createdAt }

// UpdatedAt trả về thời điểm cập nhật gần nhất.
func (i Incident) UpdatedAt() time.Time { return i.updatedAt }

// AssignedAt trả về thời điểm assign lần đầu (nil nếu chưa).
func (i Incident) AssignedAt() *time.Time { return copyTime(i.assignedAt) }

// ResolvedAt trả về thời điểm resolved (nil nếu chưa).
func (i Incident) ResolvedAt() *time.Time { return copyTime(i.resolvedAt) }

// ClosedAt trả về thời điểm closed (nil nếu chưa).
func (i Incident) ClosedAt() *time.Time { return copyTime(i.closedAt) }

func copyTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	c := *t
	return &c
}
