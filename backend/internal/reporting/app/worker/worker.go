// Package worker chứa runner nền của reporting BC: xử lý export job pending và
// chạy report schedule đến hạn. Mỗi vòng quét (Sweep/ProcessOne) idempotent.
package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/namdam97/logmon/backend/internal/reporting/domain"
	"github.com/namdam97/logmon/backend/internal/reporting/ports"
)

// _signedURLTTL là hạn của file export trên S3 (doc_v2/07: URL hết hạn 24h).
const _signedURLTTL = 24 * time.Hour

// ExportWorker lấy job pending, export dữ liệu, upload S3, đánh dấu completed.
type ExportWorker struct {
	repo     ports.ExportJobRepository
	exporter ports.Exporter
	blobs    ports.BlobStore
	clock    ports.Clock
}

// NewExportWorker tạo worker.
func NewExportWorker(repo ports.ExportJobRepository, exporter ports.Exporter, blobs ports.BlobStore, clock ports.Clock) *ExportWorker {
	return &ExportWorker{repo: repo, exporter: exporter, blobs: blobs, clock: clock}
}

// ProcessOne xử lý tối đa một job pending. processed=false nếu hàng đợi trống.
func (w *ExportWorker) ProcessOne(ctx context.Context) (bool, error) {
	job, ok, err := w.repo.ClaimNextPending(ctx)
	if err != nil {
		return false, fmt.Errorf("claim job: %w", err)
	}
	if !ok {
		return false, nil
	}
	processing, err := job.MarkProcessing()
	if err != nil {
		// Job không còn pending (đã bị claim khác) — bỏ qua, không lỗi.
		return true, nil
	}
	if err := w.repo.Update(ctx, processing); err != nil {
		return true, fmt.Errorf("mark processing: %w", err)
	}

	data, rowCount, err := w.exporter.Export(ctx, processing)
	if err != nil {
		return true, w.fail(ctx, processing, err)
	}
	key := exportKey(processing)
	if err := w.blobs.Put(ctx, key, data); err != nil {
		return true, w.fail(ctx, processing, err)
	}
	now := w.clock.Now()
	done := processing.MarkCompleted(key, rowCount, int64(len(data)), now, now.Add(_signedURLTTL))
	if err := w.repo.Update(ctx, done); err != nil {
		return true, fmt.Errorf("mark completed: %w", err)
	}
	return true, nil
}

func (w *ExportWorker) fail(ctx context.Context, job domain.ExportJob, cause error) error {
	failed := job.MarkFailed(w.clock.Now())
	if err := w.repo.Update(ctx, failed); err != nil {
		return errors.Join(cause, fmt.Errorf("mark failed: %w", err))
	}
	return cause
}

func exportKey(j domain.ExportJob) string {
	return fmt.Sprintf("exports/%s/%s.%s", j.WorkspaceID(), j.ID(), j.Format().String())
}

// ReportScheduler chạy các report schedule đã đến hạn theo cron.
type ReportScheduler struct {
	reader    ports.ScheduleReader
	repo      ports.ScheduleRepository
	cron      ports.CronScheduler
	generator ports.ReportGenerator
	delivery  ports.ReportDelivery
	clock     ports.Clock
}

// NewReportScheduler tạo scheduler.
func NewReportScheduler(reader ports.ScheduleReader, repo ports.ScheduleRepository, cron ports.CronScheduler, gen ports.ReportGenerator, delivery ports.ReportDelivery, clock ports.Clock) *ReportScheduler {
	return &ReportScheduler{reader: reader, repo: repo, cron: cron, generator: gen, delivery: delivery, clock: clock}
}

// SweepResult tóm tắt một vòng quét.
type SweepResult struct {
	Scanned int
	Ran     int
}

// Sweep chạy mọi schedule đến hạn (cron.Next(anchor) <= now). Lỗi một schedule
// không chặn các schedule khác (gom qua errors.Join).
func (s *ReportScheduler) Sweep(ctx context.Context) (SweepResult, error) {
	schedules, err := s.reader.ListEnabled(ctx)
	if err != nil {
		return SweepResult{}, fmt.Errorf("list enabled: %w", err)
	}
	now := s.clock.Now()
	res := SweepResult{Scanned: len(schedules)}
	var errs []error
	for _, sch := range schedules {
		due, err := s.isDue(sch, now)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if !due {
			continue
		}
		if err := s.run(ctx, sch, now); err != nil {
			errs = append(errs, err)
			continue
		}
		res.Ran++
	}
	return res, errors.Join(errs...)
}

func (s *ReportScheduler) isDue(sch domain.ReportSchedule, now time.Time) (bool, error) {
	anchor := sch.CreatedAt()
	if last := sch.LastRunAt(); last != nil {
		anchor = *last
	}
	next, err := s.cron.Next(sch.CronExpr(), sch.Timezone(), anchor)
	if err != nil {
		return false, fmt.Errorf("cron next %s: %w", sch.ID(), err)
	}
	return !next.After(now), nil
}

func (s *ReportScheduler) run(ctx context.Context, sch domain.ReportSchedule, now time.Time) error {
	data, err := s.generator.Generate(ctx, sch)
	if err != nil {
		return fmt.Errorf("generate %s: %w", sch.ID(), err)
	}
	if err := s.delivery.Deliver(ctx, sch, data); err != nil {
		return fmt.Errorf("deliver %s: %w", sch.ID(), err)
	}
	if err := s.repo.Update(ctx, sch.WithLastRun(now)); err != nil {
		return fmt.Errorf("update last run %s: %w", sch.ID(), err)
	}
	return nil
}
