package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/namdam97/logmon/backend/internal/incident/domain"
	"github.com/namdam97/logmon/backend/internal/incident/ports"
)

// postmortemTransitioner là phần TransitionHandler mà reminder dùng (ISP).
type postmortemTransitioner interface {
	RequirePostmortem(ctx context.Context, workspaceID, id, actor string) (domain.Incident, error)
}

// PostmortemReminderService tự chuyển incident SEV1/SEV2 đã Resolved quá `grace`
// sang PostmortemPending (doc_v2/06 §1.5 — bắt buộc postmortem). Idempotent: chỉ
// nhắm incident còn ở status Resolved; gọi lặp lại an toàn.
type PostmortemReminderService struct {
	due         ports.PostmortemDueReader
	transitions postmortemTransitioner
	clock       ports.Clock
	grace       time.Duration
}

// NewPostmortemReminderService tạo service. grace<=0 → mặc định 24h.
func NewPostmortemReminderService(due ports.PostmortemDueReader, transitions postmortemTransitioner, clock ports.Clock, grace time.Duration) *PostmortemReminderService {
	if grace <= 0 {
		grace = 24 * time.Hour
	}
	return &PostmortemReminderService{due: due, transitions: transitions, clock: clock, grace: grace}
}

// Sweep quét một lượt: chuyển các incident quá hạn sang PostmortemPending. Trả số
// incident đã chuyển + lỗi gộp (không dừng vì một incident lỗi).
func (s *PostmortemReminderService) Sweep(ctx context.Context) (int, error) {
	before := s.clock.Now().Add(-s.grace)
	incidents, err := s.due.ListResolvedNeedingPostmortem(ctx, before)
	if err != nil {
		return 0, fmt.Errorf("list resolved needing postmortem: %w", err)
	}

	flagged := 0
	var errs []error
	for _, inc := range incidents {
		_, err := s.transitions.RequirePostmortem(ctx, inc.WorkspaceID(), inc.ID().String(), "system")
		switch {
		case err == nil:
			flagged++
		case errors.Is(err, domain.ErrInvalidTransition):
			// Đã rời Resolved giữa chừng (race) — bỏ qua, không tính lỗi.
		default:
			errs = append(errs, fmt.Errorf("incident %s: %w", inc.ID().String(), err))
		}
	}
	return flagged, errors.Join(errs...)
}
