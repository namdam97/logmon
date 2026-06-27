package command

import (
	"context"
	"fmt"

	"github.com/namdam97/logmon/backend/internal/logpipeline/ports"
)

// DLQCommands xử lý retry DLQ entries sau review (không auto-replay — doc_v2/03 §6).
type DLQCommands struct {
	dlq      ports.DLQRepository
	replayer ports.DLQReplayer
	clock    Clock
}

// NewDLQCommands tạo service. replayer có thể nil (chỉ đánh dấu, không re-publish).
func NewDLQCommands(dlq ports.DLQRepository, replayer ports.DLQReplayer, clock Clock) *DLQCommands {
	return &DLQCommands{dlq: dlq, replayer: replayer, clock: clock}
}

// RetryResult tóm tắt kết quả retry hàng loạt.
type RetryResult struct {
	Retried []int64
	Failed  map[int64]string
}

// Retry replay các entry pending được chọn. Mỗi entry: load → MarkRetried →
// replay (best-effort) → persist trạng thái. Entry không pending/không tồn tại
// được ghi vào Failed, không chặn các entry khác.
func (s *DLQCommands) Retry(ctx context.Context, workspaceID string, ids []int64) (RetryResult, error) {
	res := RetryResult{Failed: map[int64]string{}}
	for _, id := range ids {
		if err := s.retryOne(ctx, workspaceID, id); err != nil {
			res.Failed[id] = err.Error()
			continue
		}
		res.Retried = append(res.Retried, id)
	}
	return res, nil
}

func (s *DLQCommands) retryOne(ctx context.Context, workspaceID string, id int64) error {
	entry, err := s.dlq.ByID(ctx, workspaceID, id)
	if err != nil {
		return err
	}
	retried, err := entry.MarkRetried(s.clock.Now())
	if err != nil {
		return err
	}
	if s.replayer != nil {
		if err := s.replayer.Replay(ctx, entry); err != nil {
			return fmt.Errorf("replay: %w", err)
		}
	}
	if err := s.dlq.UpdateStatus(ctx, workspaceID, id, retried.Status(), retried.RetryCount(), retried.RetriedAt()); err != nil {
		return fmt.Errorf("update dlq status: %w", err)
	}
	return nil
}
