package ports

import (
	"context"
	"time"

	"github.com/namdam97/logmon/backend/internal/incident/domain"
)

// Ports cho on-call & escalation (doc_v2/06 §1.4). On-call "ai trực" tính bằng
// pure function domain.WhoIsOnCall; các reader chỉ cấp dữ liệu (schedule, override,
// policy). Escalation runner dùng state repo để chỉ thông báo bậc mới (idempotent).

// ScheduleRepository là write side cho on-call schedule.
type ScheduleRepository interface {
	Save(ctx context.Context, s domain.Schedule) error
	Update(ctx context.Context, s domain.Schedule) error
}

// ScheduleReader là read side cho on-call schedule.
type ScheduleReader interface {
	ByID(ctx context.Context, id domain.ScheduleID) (domain.Schedule, error)
	List(ctx context.Context, workspaceID string) ([]domain.Schedule, error)
}

// OverrideRepository ghi override (swap/nghỉ phép).
type OverrideRepository interface {
	Save(ctx context.Context, o domain.Override) error
}

// OverrideReader đọc các override đang hiệu lực của một schedule tại thời điểm.
type OverrideReader interface {
	// ActiveForSchedule trả về override phủ thời điểm `at` của schedule.
	ActiveForSchedule(ctx context.Context, scheduleID domain.ScheduleID, at time.Time) ([]domain.Override, error)
}

// EscalationPolicyRepository ghi escalation policy.
type EscalationPolicyRepository interface {
	Save(ctx context.Context, p domain.EscalationPolicy) error
}

// EscalationPolicyReader đọc escalation policy của workspace (1 policy/workspace).
type EscalationPolicyReader interface {
	// ByWorkspace trả về policy của workspace; ErrEscalationPolicyNotFound nếu chưa có.
	ByWorkspace(ctx context.Context, workspaceID string) (domain.EscalationPolicy, error)
}

// EscalationStateReader đọc bậc escalation cao nhất đã thông báo cho incident.
type EscalationStateReader interface {
	// HighestNotified trả về bậc cao nhất đã thông báo (-1 nếu chưa có).
	HighestNotified(ctx context.Context, incidentID domain.IncidentID) (int, error)
}

// EscalationStateRepository ghi nhận một bậc escalation đã thông báo (idempotent
// theo (incident_id, level)).
type EscalationStateRepository interface {
	Record(ctx context.Context, incidentID domain.IncidentID, level int, recipient string, at time.Time) error
}

// UnackedReader liệt kê incident đang active CHƯA được ack (status open/triaged) —
// đầu vào của escalation runner.
type UnackedReader interface {
	ListUnacked(ctx context.Context) ([]domain.Incident, error)
}

// EscalationNotice là yêu cầu thông báo escalation gửi tới Notification Hub.
type EscalationNotice struct {
	IncidentID  string
	WorkspaceID string
	Title       string
	Service     string
	Severity    string
	Level       int
	Target      string // primary|secondary|team_lead
	Recipient   string
}

// EscalationNotifier gửi thông báo escalation (delivery qua Notification Hub).
type EscalationNotifier interface {
	Notify(ctx context.Context, notice EscalationNotice) error
}
