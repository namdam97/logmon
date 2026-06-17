package outbox

import (
	"context"
	"time"
)

// Processor là cổng persistence relay cần. Adapter postgres claim batch pending
// bằng FOR UPDATE SKIP LOCKED trong một TX, dispatch, rồi mark published/failed
// — tất cả trong TX đó (giữ lock tới commit, an toàn nhiều relay instance).
type Processor interface {
	// ProcessBatch claim tối đa limit event pending, dispatch từng event; ok →
	// published, lỗi → retry_count++ (đạt max → failed). Trả về (đã xử lý, số failed).
	ProcessBatch(ctx context.Context, limit int, dispatch Handler) (processed, failed int, err error)
	// OldestPendingAge trả tuổi event pending cũ nhất cho metric lag; ok=false nếu rỗng.
	OldestPendingAge(ctx context.Context) (age time.Duration, ok bool, err error)
}

// Relay là background worker quét outbox và dispatch qua Bus. Tuân chuẩn
// concurrency: stop bằng ctx hủy, chờ thoát bằng Wait() (done channel).
type Relay struct {
	proc     Processor
	dispatch Handler
	obs      Observer
	interval time.Duration
	batch    int
	done     chan struct{}
}

// RelayOption cấu hình Relay (functional options).
type RelayOption func(*Relay)

// WithInterval đặt chu kỳ poll.
func WithInterval(d time.Duration) RelayOption { return func(r *Relay) { r.interval = d } }

// WithBatchSize đặt số event tối đa mỗi batch.
func WithBatchSize(n int) RelayOption { return func(r *Relay) { r.batch = n } }

// WithObserver gắn observer metrics (mặc định NopObserver).
func WithObserver(o Observer) RelayOption { return func(r *Relay) { r.obs = o } }

// NewRelay tạo relay; dispatch thường là bus.Dispatch.
func NewRelay(proc Processor, dispatch Handler, opts ...RelayOption) *Relay {
	r := &Relay{
		proc:     proc,
		dispatch: dispatch,
		obs:      NopObserver{},
		interval: DefaultPollInterval,
		batch:    DefaultBatchSize,
		done:     make(chan struct{}),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Run chạy tới khi ctx hủy, rồi đóng done. Chạy một tick ngay khi bắt đầu.
func (r *Relay) Run(ctx context.Context) {
	defer close(r.done)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		r.tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Wait chặn tới khi Run thoát hẳn.
func (r *Relay) Wait() { <-r.done }

// tick xử lý hết các batch pending hiện có (drain) và cập nhật metric lag.
func (r *Relay) tick(ctx context.Context) {
	if age, ok, err := r.proc.OldestPendingAge(ctx); err == nil && ok {
		r.obs.ObserveLag(age.Seconds())
	}
	for {
		processed, failed, err := r.proc.ProcessBatch(ctx, r.batch, r.dispatch)
		if failed > 0 {
			r.obs.IncFailed(failed)
		}
		// Hết việc / lỗi / batch chưa đầy (đã drain) / ctx hủy → dừng vòng drain.
		if err != nil || processed == 0 || processed < r.batch || ctx.Err() != nil {
			return
		}
	}
}
