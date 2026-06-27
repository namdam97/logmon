package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/namdam97/logmon/backend/internal/incident/domain"
	"github.com/namdam97/logmon/backend/internal/incident/ports"
)

// EscalationService quét các incident chưa ack và escalate bậc mới tới hạn
// (doc_v2/06 §1.4). Idempotent: chỉ thông báo bậc cao hơn "đã thông báo" (lưu ở
// EscalationStateRepository), nên gọi lặp lại định kỳ là an toàn. Lỗi từng
// incident được gom lại, không làm hỏng cả lượt quét.
type EscalationService struct {
	unacked   ports.UnackedReader
	policy    ports.EscalationPolicyReader
	schedule  ports.ScheduleReader
	overrides ports.OverrideReader
	state     ports.EscalationStateReader
	stateRepo ports.EscalationStateRepository
	notifier  ports.EscalationNotifier
	clock     ports.Clock
}

// NewEscalationService tạo service với dependency được inject.
func NewEscalationService(
	unacked ports.UnackedReader,
	policy ports.EscalationPolicyReader,
	schedule ports.ScheduleReader,
	overrides ports.OverrideReader,
	state ports.EscalationStateReader,
	stateRepo ports.EscalationStateRepository,
	notifier ports.EscalationNotifier,
	clock ports.Clock,
) *EscalationService {
	return &EscalationService{
		unacked: unacked, policy: policy, schedule: schedule, overrides: overrides,
		state: state, stateRepo: stateRepo, notifier: notifier, clock: clock,
	}
}

// SweepResult tóm tắt một lượt quét escalation.
type SweepResult struct {
	Scanned   int // số incident chưa ack đã xét
	Escalated int // số thông báo escalation đã gửi
}

// Sweep quét một lượt: với mỗi incident chưa ack, escalate bậc mới tới hạn.
// Trả lỗi gộp (errors.Join) nếu có incident lỗi; vẫn xử lý hết các incident khác.
func (s *EscalationService) Sweep(ctx context.Context) (SweepResult, error) {
	now := s.clock.Now()
	incidents, err := s.unacked.ListUnacked(ctx)
	if err != nil {
		return SweepResult{}, fmt.Errorf("list unacked: %w", err)
	}

	var res SweepResult
	var errs []error
	for _, inc := range incidents {
		res.Scanned++
		sent, err := s.escalateOne(ctx, inc, now)
		res.Escalated += sent
		if err != nil {
			errs = append(errs, fmt.Errorf("incident %s: %w", inc.ID().String(), err))
		}
	}
	return res, errors.Join(errs...)
}

// escalateOne escalate một incident; trả số thông báo đã gửi.
func (s *EscalationService) escalateOne(ctx context.Context, inc domain.Incident, now time.Time) (int, error) {
	policy, err := s.policy.ByWorkspace(ctx, inc.WorkspaceID())
	if err != nil {
		if errors.Is(err, domain.ErrEscalationPolicyNotFound) {
			return 0, nil // workspace chưa cấu hình escalation → bỏ qua
		}
		return 0, fmt.Errorf("load policy: %w", err)
	}

	elapsed := now.Sub(inc.CreatedAt())
	highestDue := policy.HighestDueLevel(elapsed)
	if highestDue < 0 {
		return 0, nil
	}
	highestNotified, err := s.state.HighestNotified(ctx, inc.ID())
	if err != nil {
		return 0, fmt.Errorf("read escalation state: %w", err)
	}
	if highestDue <= highestNotified {
		return 0, nil // không có bậc mới
	}

	oncall := s.resolveOnCall(ctx, inc.WorkspaceID(), now)
	levels := policy.Levels()

	sent := 0
	for level := highestNotified + 1; level <= highestDue; level++ {
		recipient, ok := policy.ResolveTarget(level, oncall)
		target := levels[level].Target().String()
		if ok {
			notice := ports.EscalationNotice{
				IncidentID:  inc.ID().String(),
				WorkspaceID: inc.WorkspaceID(),
				Title:       inc.Title(),
				Service:     inc.Service(),
				Severity:    inc.Severity().Label(),
				Level:       level,
				Target:      target,
				Recipient:   recipient,
			}
			if err := s.notifier.Notify(ctx, notice); err != nil {
				// Không advance state khi gửi lỗi → lượt sau thử lại bậc này.
				return sent, fmt.Errorf("notify level %d: %w", level, err)
			}
			sent++
		}
		// Advance con trỏ kể cả khi không resolve được người nhận (vd secondary
		// rỗng) để không kẹt lại bậc đó mãi.
		if err := s.stateRepo.Record(ctx, inc.ID(), level, recipient, now); err != nil {
			return sent, fmt.Errorf("record level %d: %w", level, err)
		}
	}
	return sent, nil
}

// resolveOnCall tính on-call hiện tại từ schedule đầu tiên của workspace; trả
// OnCall rỗng nếu workspace chưa có schedule (chỉ bậc team_lead resolve được).
func (s *EscalationService) resolveOnCall(ctx context.Context, workspaceID string, at time.Time) domain.OnCall {
	schedules, err := s.schedule.List(ctx, workspaceID)
	if err != nil || len(schedules) == 0 {
		return domain.OnCall{}
	}
	sched := schedules[0]
	active, err := s.overrides.ActiveForSchedule(ctx, sched.ID(), at)
	if err != nil {
		return domain.WhoIsOnCall(sched, nil, at)
	}
	return domain.WhoIsOnCall(sched, active, at)
}
