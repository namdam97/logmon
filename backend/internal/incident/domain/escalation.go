package domain

import (
	"strings"
	"time"
)

// Escalation policy (doc_v2/06 §1.4): primary → secondary → team_lead. Mỗi level
// có timeout; nếu incident chưa được ack (chưa Assigned/Mitigating) trong timeout
// thì escalate sang level kế. Mặc định: primary(15m) → secondary(30m) → team_lead(1h).
//
// Mọi tính toán "level nào tới hạn tại elapsed T" là PURE — runner ở app/adapter
// chỉ persist level đã thông báo và gọi Notification Hub.

const _maxEscalationLevels = 10

// EscalationTarget là đích của một level: primary/secondary (suy từ on-call) hoặc
// team_lead (người cố định trong policy).
type EscalationTarget struct {
	value string
}

// Ba đích escalation.
var (
	TargetPrimary   = EscalationTarget{"primary"}
	TargetSecondary = EscalationTarget{"secondary"}
	TargetTeamLead  = EscalationTarget{"team_lead"}
)

var _validTargets = map[string]EscalationTarget{
	TargetPrimary.value:   TargetPrimary,
	TargetSecondary.value: TargetSecondary,
	TargetTeamLead.value:  TargetTeamLead,
}

// NewEscalationTarget validate và bọc một target string.
func NewEscalationTarget(raw string) (EscalationTarget, error) {
	if t, ok := _validTargets[raw]; ok {
		return t, nil
	}
	return EscalationTarget{}, newValidationError("target", "must be one of primary|secondary|team_lead")
}

// String trả về biểu diễn chuỗi của target.
func (t EscalationTarget) String() string { return t.value }

// EscalationLevel là một bậc: đích + timeout chờ ack trước khi escalate bậc kế.
type EscalationLevel struct {
	target  EscalationTarget
	timeout time.Duration
}

// NewEscalationLevel tạo một bậc escalation hợp lệ.
func NewEscalationLevel(target string, timeout time.Duration) (EscalationLevel, error) {
	t, err := NewEscalationTarget(target)
	if err != nil {
		return EscalationLevel{}, err
	}
	if timeout <= 0 {
		return EscalationLevel{}, newValidationError("timeout", "must be positive")
	}
	return EscalationLevel{target: t, timeout: timeout}, nil
}

// Target trả về đích của bậc.
func (l EscalationLevel) Target() EscalationTarget { return l.target }

// Timeout trả về thời gian chờ ack của bậc.
func (l EscalationLevel) Timeout() time.Duration { return l.timeout }

// EscalationPolicy là chuỗi bậc escalation theo thứ tự + team lead cố định.
type EscalationPolicy struct {
	id          string
	workspaceID string
	name        string
	levels      []EscalationLevel
	teamLead    string
}

// NewEscalationPolicyInput gom tham số tạo policy.
type NewEscalationPolicyInput struct {
	ID          string
	WorkspaceID string
	Name        string
	Levels      []EscalationLevel
	TeamLead    string
}

// NewEscalationPolicy tạo policy sau khi validate. Nếu có bậc team_lead thì
// teamLead bắt buộc khác rỗng (không thì không resolve được người nhận).
func NewEscalationPolicy(in NewEscalationPolicyInput) (EscalationPolicy, error) {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		return EscalationPolicy{}, newValidationError("id", "must not be empty")
	}
	if strings.TrimSpace(in.WorkspaceID) == "" {
		return EscalationPolicy{}, newValidationError("workspace_id", "must not be empty")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return EscalationPolicy{}, newValidationError("name", "must not be empty")
	}
	if len(in.Levels) == 0 {
		return EscalationPolicy{}, newValidationError("levels", "must have at least one level")
	}
	if len(in.Levels) > _maxEscalationLevels {
		return EscalationPolicy{}, newValidationError("levels", "too many levels")
	}
	teamLead := strings.TrimSpace(in.TeamLead)
	levels := make([]EscalationLevel, len(in.Levels))
	copy(levels, in.Levels)
	for _, lv := range levels {
		if lv.target == TargetTeamLead && teamLead == "" {
			return EscalationPolicy{}, newValidationError("team_lead", "required when a level targets team_lead")
		}
	}
	return EscalationPolicy{
		id:          id,
		workspaceID: in.WorkspaceID,
		name:        name,
		levels:      levels,
		teamLead:    teamLead,
	}, nil
}

// DefaultEscalationPolicy trả về policy chuẩn theo doc_v2/06 §1.4:
// primary(15m) → secondary(30m) → team_lead(1h).
func DefaultEscalationPolicy(id, workspaceID, name, teamLead string) (EscalationPolicy, error) {
	levels := []EscalationLevel{
		{target: TargetPrimary, timeout: 15 * time.Minute},
		{target: TargetSecondary, timeout: 30 * time.Minute},
		{target: TargetTeamLead, timeout: 60 * time.Minute},
	}
	return NewEscalationPolicy(NewEscalationPolicyInput{
		ID:          id,
		WorkspaceID: workspaceID,
		Name:        name,
		Levels:      levels,
		TeamLead:    teamLead,
	})
}

// ID trả về định danh policy.
func (p EscalationPolicy) ID() string { return p.id }

// WorkspaceID trả về workspace sở hữu policy.
func (p EscalationPolicy) WorkspaceID() string { return p.workspaceID }

// Name trả về tên policy.
func (p EscalationPolicy) Name() string { return p.name }

// TeamLead trả về người resolve cho TargetTeamLead.
func (p EscalationPolicy) TeamLead() string { return p.teamLead }

// Levels trả về BẢN SAO danh sách bậc.
func (p EscalationPolicy) Levels() []EscalationLevel {
	out := make([]EscalationLevel, len(p.levels))
	copy(out, p.levels)
	return out
}

// notifyMark trả về thời điểm (tính từ lúc tạo incident) mà bậc index `i` được
// thông báo = tổng timeout các bậc đứng trước. Bậc 0 luôn ở mark 0.
func (p EscalationPolicy) notifyMark(i int) time.Duration {
	var mark time.Duration
	for k := 0; k < i && k < len(p.levels); k++ {
		mark += p.levels[k].timeout
	}
	return mark
}

// DueLevelIndexes trả về index các bậc đã tới hạn thông báo tại `elapsed` (kể từ
// khi tạo incident). Bậc i tới hạn khi elapsed >= tổng timeout các bậc trước.
// PURE — runner so sánh với "đã thông báo" để chỉ gửi bậc mới.
func (p EscalationPolicy) DueLevelIndexes(elapsed time.Duration) []int {
	out := make([]int, 0, len(p.levels))
	for i := range p.levels {
		if elapsed >= p.notifyMark(i) {
			out = append(out, i)
		} else {
			break
		}
	}
	return out
}

// HighestDueLevel trả về index bậc cao nhất đã tới hạn tại `elapsed`, hoặc -1 nếu
// chưa có bậc nào (elapsed < 0).
func (p EscalationPolicy) HighestDueLevel(elapsed time.Duration) int {
	due := p.DueLevelIndexes(elapsed)
	if len(due) == 0 {
		return -1
	}
	return due[len(due)-1]
}

// ResolveTarget ánh xạ bậc index `i` sang người nhận cụ thể dựa trên on-call hiện
// tại. Trả về (recipient, ok=false) nếu index sai hoặc không resolve được
// (vd secondary rỗng do schedule chỉ có 1 người).
func (p EscalationPolicy) ResolveTarget(i int, oncall OnCall) (string, bool) {
	if i < 0 || i >= len(p.levels) {
		return "", false
	}
	switch p.levels[i].target {
	case TargetPrimary:
		return p.levels[i].targetOrEmpty(oncall.Primary)
	case TargetSecondary:
		return p.levels[i].targetOrEmpty(oncall.Secondary)
	case TargetTeamLead:
		return p.levels[i].targetOrEmpty(p.teamLead)
	default:
		return "", false
	}
}

// targetOrEmpty trả về (v, true) nếu v khác rỗng, ngược lại ("", false).
func (l EscalationLevel) targetOrEmpty(v string) (string, bool) {
	if strings.TrimSpace(v) == "" {
		return "", false
	}
	return v, true
}

// ReconstructEscalationPolicy dựng lại policy từ persistence (đã validate khi ghi).
func ReconstructEscalationPolicy(id, workspaceID, name, teamLead string, levels []EscalationLevel) EscalationPolicy {
	cp := make([]EscalationLevel, len(levels))
	copy(cp, levels)
	return EscalationPolicy{
		id:          id,
		workspaceID: workspaceID,
		name:        name,
		levels:      cp,
		teamLead:    teamLead,
	}
}

// ReconstructEscalationLevel dựng lại một bậc từ persistence.
func ReconstructEscalationLevel(target string, timeout time.Duration) EscalationLevel {
	t := TargetPrimary
	if v, ok := _validTargets[target]; ok {
		t = v
	}
	return EscalationLevel{target: t, timeout: timeout}
}
