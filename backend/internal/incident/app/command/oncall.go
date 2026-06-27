package command

import (
	"context"
	"fmt"
	"time"

	"github.com/namdam97/logmon/backend/internal/incident/domain"
	"github.com/namdam97/logmon/backend/internal/incident/ports"
)

// On-call write-side use cases: tạo schedule, tạo override, tạo escalation policy.
// Đơn giản (không outbox/event) — persist trực tiếp, validate ở domain.

// CreateScheduleInput là dữ liệu vào tạo on-call schedule.
type CreateScheduleInput struct {
	WorkspaceID  string
	Name         string
	Rotation     string // daily | weekly
	Participants []string
	Timezone     string // IANA; rỗng → UTC
	HandoffHour  int
	HandoffMin   int
	StartDate    time.Time
}

// CreateScheduleHandler tạo on-call schedule mới.
type CreateScheduleHandler struct {
	repo  ports.ScheduleRepository
	ids   ports.IDGenerator
	clock ports.Clock
}

// NewCreateScheduleHandler tạo handler với dependency được inject.
func NewCreateScheduleHandler(repo ports.ScheduleRepository, ids ports.IDGenerator, clock ports.Clock) *CreateScheduleHandler {
	return &CreateScheduleHandler{repo: repo, ids: ids, clock: clock}
}

// Handle tạo schedule. Trả domain.ValidationError nếu input sai.
func (h *CreateScheduleHandler) Handle(ctx context.Context, in CreateScheduleInput) (domain.Schedule, error) {
	s, err := domain.NewSchedule(domain.NewScheduleInput{
		ID:           h.ids.NewID(),
		WorkspaceID:  in.WorkspaceID,
		Name:         in.Name,
		Rotation:     in.Rotation,
		Participants: in.Participants,
		Timezone:     in.Timezone,
		HandoffHour:  in.HandoffHour,
		HandoffMin:   in.HandoffMin,
		StartDate:    in.StartDate,
		Now:          h.clock.Now(),
	})
	if err != nil {
		return domain.Schedule{}, err
	}
	if err := h.repo.Save(ctx, s); err != nil {
		return domain.Schedule{}, fmt.Errorf("save schedule: %w", err)
	}
	return s, nil
}

// CreateOverrideInput là dữ liệu vào tạo override (swap/nghỉ phép).
type CreateOverrideInput struct {
	ScheduleID  string
	Participant string
	StartAt     time.Time
	EndAt       time.Time
}

// CreateOverrideHandler tạo override on-call.
type CreateOverrideHandler struct {
	repo     ports.OverrideRepository
	schedule ports.ScheduleReader
	ids      ports.IDGenerator
	clock    ports.Clock
}

// NewCreateOverrideHandler tạo handler với dependency được inject.
func NewCreateOverrideHandler(repo ports.OverrideRepository, schedule ports.ScheduleReader, ids ports.IDGenerator, clock ports.Clock) *CreateOverrideHandler {
	return &CreateOverrideHandler{repo: repo, schedule: schedule, ids: ids, clock: clock}
}

// Handle tạo override sau khi xác nhận schedule tồn tại.
func (h *CreateOverrideHandler) Handle(ctx context.Context, in CreateOverrideInput) (domain.Override, error) {
	sid, err := domain.NewScheduleID(in.ScheduleID)
	if err != nil {
		return domain.Override{}, err
	}
	if _, err := h.schedule.ByID(ctx, sid); err != nil {
		return domain.Override{}, err // ErrScheduleNotFound được truyền lên handler HTTP
	}
	o, err := domain.NewOverride(domain.NewOverrideInput{
		ID:          h.ids.NewID(),
		ScheduleID:  in.ScheduleID,
		Participant: in.Participant,
		StartAt:     in.StartAt,
		EndAt:       in.EndAt,
		Now:         h.clock.Now(),
	})
	if err != nil {
		return domain.Override{}, err
	}
	if err := h.repo.Save(ctx, o); err != nil {
		return domain.Override{}, fmt.Errorf("save override: %w", err)
	}
	return o, nil
}

// CreateEscalationPolicyInput là dữ liệu vào tạo escalation policy.
type CreateEscalationPolicyInput struct {
	WorkspaceID string
	Name        string
	TeamLead    string
	// Levels rỗng → dùng policy mặc định primary(15m)→secondary(30m)→team_lead(1h).
	Levels []EscalationLevelInput
}

// EscalationLevelInput mô tả một bậc escalation từ API.
type EscalationLevelInput struct {
	Target         string
	TimeoutMinutes int
}

// CreateEscalationPolicyHandler tạo escalation policy cho workspace.
type CreateEscalationPolicyHandler struct {
	repo ports.EscalationPolicyRepository
	ids  ports.IDGenerator
}

// NewCreateEscalationPolicyHandler tạo handler với dependency được inject.
func NewCreateEscalationPolicyHandler(repo ports.EscalationPolicyRepository, ids ports.IDGenerator) *CreateEscalationPolicyHandler {
	return &CreateEscalationPolicyHandler{repo: repo, ids: ids}
}

// Handle tạo policy. Levels rỗng → policy mặc định theo doc_v2/06 §1.4.
func (h *CreateEscalationPolicyHandler) Handle(ctx context.Context, in CreateEscalationPolicyInput) (domain.EscalationPolicy, error) {
	var (
		p   domain.EscalationPolicy
		err error
	)
	if len(in.Levels) == 0 {
		p, err = domain.DefaultEscalationPolicy(h.ids.NewID(), in.WorkspaceID, in.Name, in.TeamLead)
	} else {
		levels := make([]domain.EscalationLevel, 0, len(in.Levels))
		for _, lv := range in.Levels {
			level, lerr := domain.NewEscalationLevel(lv.Target, time.Duration(lv.TimeoutMinutes)*time.Minute)
			if lerr != nil {
				return domain.EscalationPolicy{}, lerr
			}
			levels = append(levels, level)
		}
		p, err = domain.NewEscalationPolicy(domain.NewEscalationPolicyInput{
			ID:          h.ids.NewID(),
			WorkspaceID: in.WorkspaceID,
			Name:        in.Name,
			Levels:      levels,
			TeamLead:    in.TeamLead,
		})
	}
	if err != nil {
		return domain.EscalationPolicy{}, err
	}
	if err := h.repo.Save(ctx, p); err != nil {
		return domain.EscalationPolicy{}, fmt.Errorf("save escalation policy: %w", err)
	}
	return p, nil
}
