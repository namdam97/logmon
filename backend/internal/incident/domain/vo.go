package domain

import "strings"

// IncidentID là value object định danh incident (UUID dạng chuỗi).
type IncidentID struct {
	value string
}

// NewIncidentID validate và bọc định danh incident không rỗng.
func NewIncidentID(raw string) (IncidentID, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return IncidentID{}, newValidationError("id", "must not be empty")
	}
	return IncidentID{value: v}, nil
}

// String trả về biểu diễn chuỗi của IncidentID.
func (id IncidentID) String() string { return id.value }

// Status là value object cho trạng thái incident trong state machine 7 trạng thái.
type Status struct {
	value string
}

// Bảy trạng thái lifecycle (doc_v2/06 §1.1).
var (
	StatusOpen              = Status{"open"}
	StatusTriaged           = Status{"triaged"}
	StatusAssigned          = Status{"assigned"}
	StatusMitigating        = Status{"mitigating"}
	StatusResolved          = Status{"resolved"}
	StatusPostmortemPending = Status{"postmortem_pending"}
	StatusClosed            = Status{"closed"}
)

var _validStatuses = map[string]Status{
	StatusOpen.value:              StatusOpen,
	StatusTriaged.value:           StatusTriaged,
	StatusAssigned.value:          StatusAssigned,
	StatusMitigating.value:        StatusMitigating,
	StatusResolved.value:          StatusResolved,
	StatusPostmortemPending.value: StatusPostmortemPending,
	StatusClosed.value:            StatusClosed,
}

// NewStatus validate và bọc một status string (dùng khi hydrate từ DB).
func NewStatus(raw string) (Status, error) {
	if s, ok := _validStatuses[raw]; ok {
		return s, nil
	}
	return Status{}, newValidationError("status", "unknown incident status")
}

// String trả về biểu diễn chuỗi của status.
func (s Status) String() string { return s.value }

// IsActive cho biết incident còn đang xử lý (chưa resolved/closed) — dùng cho
// gauge "open incidents" và dedup auto-create.
func (s Status) IsActive() bool {
	switch s {
	case StatusOpen, StatusTriaged, StatusAssigned, StatusMitigating:
		return true
	default:
		return false
	}
}

// Severity là value object cho mức độ nghiêm trọng (doc_v2/06 §1.2). Zero value
// nghĩa là CHƯA phân loại (incident vừa Open, set severity khi Triage).
type Severity struct {
	value string
}

// Bốn mức severity. SEV1 nặng nhất → SEV4 không ảnh hưởng user.
var (
	SEV1 = Severity{"SEV1"}
	SEV2 = Severity{"SEV2"}
	SEV3 = Severity{"SEV3"}
	SEV4 = Severity{"SEV4"}
)

var _validSeverities = map[string]Severity{
	SEV1.value: SEV1,
	SEV2.value: SEV2,
	SEV3.value: SEV3,
	SEV4.value: SEV4,
}

// NewSeverity validate và bọc một severity string (SEV1..SEV4).
func NewSeverity(raw string) (Severity, error) {
	if s, ok := _validSeverities[raw]; ok {
		return s, nil
	}
	return Severity{}, newValidationError("severity", "must be one of SEV1|SEV2|SEV3|SEV4")
}

// String trả về biểu diễn chuỗi của severity ("" nếu chưa phân loại).
func (s Severity) String() string { return s.value }

// IsZero cho biết severity chưa được phân loại.
func (s Severity) IsZero() bool { return s.value == "" }

// RequiresPostmortem báo SEV1/SEV2 bắt buộc postmortem (doc_v2/06 §1.5).
func (s Severity) RequiresPostmortem() bool { return s == SEV1 || s == SEV2 }

// Label trả về nhãn severity dùng cho Prometheus metric — "untriaged" nếu chưa
// phân loại (giữ cardinality thấp: 5 giá trị cố định).
func (s Severity) Label() string {
	if s.value == "" {
		return "untriaged"
	}
	return s.value
}

// Source là value object cho nguồn tạo incident (manual / alert / slo budget).
type Source struct {
	value string
}

// Các nguồn tạo incident.
var (
	SourceManual    = Source{"manual"}
	SourceAlert     = Source{"alert"}
	SourceSLOBudget = Source{"slo_budget"}
)

var _validSources = map[string]Source{
	SourceManual.value:    SourceManual,
	SourceAlert.value:     SourceAlert,
	SourceSLOBudget.value: SourceSLOBudget,
}

// NewSource validate và bọc một source string.
func NewSource(raw string) (Source, error) {
	if s, ok := _validSources[raw]; ok {
		return s, nil
	}
	return Source{}, newValidationError("source", "must be one of manual|alert|slo_budget")
}

// String trả về biểu diễn chuỗi của source.
func (s Source) String() string { return s.value }
