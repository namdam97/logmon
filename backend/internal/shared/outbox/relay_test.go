package outbox_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/shared/outbox"
)

// fakeProcessor mô phỏng Processor: trả về số event "pending" giảm dần mỗi lần
// ProcessBatch, đếm số lần gọi.
type fakeProcessor struct {
	mu        sync.Mutex
	pending   int // số event chưa xử lý
	failPerB  int // số failed báo về mỗi batch
	calls     int
	lagSecond float64
	hasLag    bool
}

func (f *fakeProcessor) ProcessBatch(_ context.Context, limit int, _ outbox.Handler) (int, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	n := limit
	if f.pending < n {
		n = f.pending
	}
	f.pending -= n
	return n, f.failPerB, nil
}

func (f *fakeProcessor) OldestPendingAge(context.Context) (time.Duration, bool, error) {
	return time.Duration(f.lagSecond * float64(time.Second)), f.hasLag, nil
}

func (f *fakeProcessor) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// recordObserver ghi lại lag + failed cho assertion.
type recordObserver struct {
	mu        sync.Mutex
	lastLag   float64
	failedSum int
}

func (o *recordObserver) ObserveLag(s float64) { o.mu.Lock(); o.lastLag = s; o.mu.Unlock() }
func (o *recordObserver) IncFailed(n int)      { o.mu.Lock(); o.failedSum += n; o.mu.Unlock() }

func TestRelayDrainsAndStops(t *testing.T) {
	// 250 pending, batch 100 → tick đầu drain 3 batch (100,100,50).
	proc := &fakeProcessor{pending: 250, lagSecond: 12, hasLag: true}
	obs := &recordObserver{}
	relay := outbox.NewRelay(proc, func(context.Context, outbox.Event) error { return nil },
		outbox.WithBatchSize(100), outbox.WithInterval(time.Hour), outbox.WithObserver(obs))

	ctx, cancel := context.WithCancel(context.Background())
	go relay.Run(ctx)

	require.Eventually(t, func() bool { return proc.callCount() >= 3 }, time.Second, 5*time.Millisecond)
	cancel()
	relay.Wait() // không treo → stop/done đúng

	obs.mu.Lock()
	require.Equal(t, float64(12), obs.lastLag)
	obs.mu.Unlock()
}

func TestRelayReportsFailed(t *testing.T) {
	proc := &fakeProcessor{pending: 5, failPerB: 2}
	obs := &recordObserver{}
	relay := outbox.NewRelay(proc, func(context.Context, outbox.Event) error { return nil },
		outbox.WithBatchSize(100), outbox.WithInterval(time.Hour), outbox.WithObserver(obs))

	ctx, cancel := context.WithCancel(context.Background())
	go relay.Run(ctx)
	require.Eventually(t, func() bool { return proc.callCount() >= 1 }, time.Second, 5*time.Millisecond)
	cancel()
	relay.Wait()

	obs.mu.Lock()
	require.GreaterOrEqual(t, obs.failedSum, 2)
	obs.mu.Unlock()
}
