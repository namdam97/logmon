// Package snapshot chứa budget snapshot job của slo BC: định kỳ query
// Prometheus/Thanos, tính budget remaining + burn rates, ghi slo_snapshots (read
// model cho API/UI), và phát BudgetExhausted khi budget tụt dưới ngưỡng (doc_v2/05 §4.3).
package snapshot

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/namdam97/logmon/backend/internal/slo/domain"
	"github.com/namdam97/logmon/backend/internal/slo/ports"
)

const (
	// _budgetExhaustedPercent: ngưỡng phát BudgetExhausted (budget còn < 10%).
	_budgetExhaustedPercent = 10.0
	// _defaultInterval: chu kỳ snapshot mặc định (doc_v2/05 §4.3).
	_defaultInterval = 5 * time.Minute
)

// Logger là interface log tối giản (ISP) — inject shared logger.
type Logger interface {
	Info(msg string, kv ...any)
	Error(msg string, kv ...any)
}

// Job tính + ghi budget snapshot định kỳ.
type Job struct {
	reader    ports.SLOReader
	querier   ports.MetricsQuerier
	snapshots ports.SnapshotRepository
	tx        ports.TxManager
	publisher ports.EventPublisher
	clock     ports.Clock
	log       Logger
	interval  time.Duration
}

// NewJob tạo job với dependency được inject.
func NewJob(reader ports.SLOReader, querier ports.MetricsQuerier, snapshots ports.SnapshotRepository, tx ports.TxManager, publisher ports.EventPublisher, clock ports.Clock, log Logger) *Job {
	return &Job{
		reader:    reader,
		querier:   querier,
		snapshots: snapshots,
		tx:        tx,
		publisher: publisher,
		clock:     clock,
		log:       log,
		interval:  _defaultInterval,
	}
}

// Run chạy vòng lặp snapshot tới khi ctx bị huỷ (stop signal). Chạy ngay 1 lần,
// rồi theo ticker. Lỗi 1 vòng được log, không dừng vòng lặp.
func (j *Job) Run(ctx context.Context) {
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()

	j.runOnceLogged(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			j.runOnceLogged(ctx)
		}
	}
}

func (j *Job) runOnceLogged(ctx context.Context) {
	if err := j.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
		j.log.Error("slo snapshot job", "error", err)
	}
}

// RunOnce tính snapshot cho mọi SLO một lần. Lỗi/query rỗng của 1 SLO được bỏ
// qua (log), không chặn các SLO khác.
func (j *Job) RunOnce(ctx context.Context) error {
	slos, err := j.reader.ListAll(ctx)
	if err != nil {
		return fmt.Errorf("list slos: %w", err)
	}
	for _, s := range slos {
		if err := j.process(ctx, s); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			j.log.Error("snapshot slo", "slo", s.ID().String(), "error", err)
		}
	}
	return nil
}

func (j *Job) process(ctx context.Context, s domain.SLO) error {
	windowStr := fmt.Sprintf("%dd", s.WindowDays())
	ratioWindow, err := j.querier.QueryScalar(ctx, s.ErrorRatioQuery(windowStr))
	if errors.Is(err, domain.ErrNoData) {
		return nil // chưa có traffic — bỏ qua, không ghi snapshot rỗng
	}
	if err != nil {
		return fmt.Errorf("query window ratio: %w", err)
	}
	ratio1h, err := j.queryRatio(ctx, s, "1h")
	if err != nil {
		return err
	}
	ratio6h, err := j.queryRatio(ctx, s, "6h")
	if err != nil {
		return err
	}
	ratio24h, err := j.queryRatio(ctx, s, "24h")
	if err != nil {
		return err
	}

	budget := s.ErrorBudget()
	budgetRemainingPercent := (1 - ratioWindow/budget) * 100

	snap := domain.NewSnapshot(domain.NewSnapshotInput{
		SLOID:                  s.ID(),
		CurrentSLI:             1 - ratioWindow,
		BudgetRemainingPercent: budgetRemainingPercent,
		BurnRate1h:             ratio1h / budget,
		BurnRate6h:             ratio6h / budget,
		BurnRate24h:            ratio24h / budget,
		RecordedAt:             j.clock.Now(),
	})

	// Edge-trigger: chỉ phát BudgetExhausted khi VỪA tụt dưới ngưỡng (tránh spam
	// mỗi 5 phút). prev không có ⇒ coi như đang khoẻ (100%).
	prevRemaining := 100.0
	if prev, err := j.snapshots.Latest(ctx, s.ID()); err == nil {
		prevRemaining = prev.BudgetRemainingPercent()
	} else if !errors.Is(err, domain.ErrSnapshotNotFound) {
		return fmt.Errorf("read prev snapshot: %w", err)
	}

	if err := j.snapshots.Save(ctx, snap); err != nil {
		return fmt.Errorf("save snapshot: %w", err)
	}

	if prevRemaining >= _budgetExhaustedPercent && budgetRemainingPercent < _budgetExhaustedPercent {
		if err := j.emitExhausted(ctx, s, budgetRemainingPercent); err != nil {
			return fmt.Errorf("emit budget exhausted: %w", err)
		}
	}
	return nil
}

// queryRatio query error ratio cửa sổ ngắn; query rỗng ⇒ 0 (chưa có lỗi).
func (j *Job) queryRatio(ctx context.Context, s domain.SLO, window string) (float64, error) {
	v, err := j.querier.QueryScalar(ctx, s.ErrorRatioQuery(window))
	if errors.Is(err, domain.ErrNoData) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("query ratio %s: %w", window, err)
	}
	return v, nil
}

func (j *Job) emitExhausted(ctx context.Context, s domain.SLO, remaining float64) error {
	return j.tx.WithinTx(ctx, func(ctx context.Context) error {
		payload := domain.BudgetExhaustedPayload{
			SLOID:                  s.ID().String(),
			WorkspaceID:            s.WorkspaceID(),
			Service:                s.Service(),
			BudgetRemainingPercent: remaining,
		}
		return j.publisher.Publish(ctx, domain.AggregateType, s.ID().String(), domain.EventBudgetExhausted, payload)
	})
}
