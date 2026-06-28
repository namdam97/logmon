package worker_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/reporting/app/worker"
	"github.com/namdam97/logmon/backend/internal/reporting/domain"
)

// ---- fakes ----

type fakeJobStore struct {
	pending []domain.ExportJob
	updated map[string]domain.ExportJob
}

func newFakeJobStore() *fakeJobStore {
	return &fakeJobStore{updated: map[string]domain.ExportJob{}}
}

func (s *fakeJobStore) Save(context.Context, domain.ExportJob) error { return nil }
func (s *fakeJobStore) ByID(context.Context, string, string) (domain.ExportJob, error) {
	return domain.ExportJob{}, domain.ErrExportJobNotFound
}
func (s *fakeJobStore) Update(_ context.Context, j domain.ExportJob) error {
	s.updated[j.ID()] = j
	return nil
}
func (s *fakeJobStore) ClaimNextPending(context.Context) (domain.ExportJob, bool, error) {
	if len(s.pending) == 0 {
		return domain.ExportJob{}, false, nil
	}
	j := s.pending[0]
	s.pending = s.pending[1:]
	return j, true, nil
}

type fakeExporter struct {
	data []byte
	rows int64
	err  error
}

func (f fakeExporter) Export(context.Context, domain.ExportJob) ([]byte, int64, error) {
	return f.data, f.rows, f.err
}

type fakeBlobs struct {
	put map[string][]byte
	err error
}

func newFakeBlobs() *fakeBlobs { return &fakeBlobs{put: map[string][]byte{}} }
func (f *fakeBlobs) Put(_ context.Context, key string, data []byte) error {
	if f.err != nil {
		return f.err
	}
	f.put[key] = data
	return nil
}
func (f *fakeBlobs) SignedURL(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://s3/" + key, nil
}

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func pendingJob(t *testing.T, id string) domain.ExportJob {
	t.Helper()
	j, err := domain.NewExportJob(domain.NewJobInput{
		ID: id, WorkspaceID: "ws-1", UserID: "u-1", ExportType: domain.ExportLogs,
		Format: domain.FormatCSV, Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)
	return j
}

// ---- export worker tests ----

func TestExportWorkerSuccess(t *testing.T) {
	store := newFakeJobStore()
	store.pending = []domain.ExportJob{pendingJob(t, "j-1")}
	blobs := newFakeBlobs()
	w := worker.NewExportWorker(store, fakeExporter{data: []byte("a,b,c"), rows: 3}, blobs, fixedClock{t: time.Unix(100, 0).UTC()})

	processed, err := w.ProcessOne(context.Background())
	require.NoError(t, err)
	require.True(t, processed)
	require.Equal(t, domain.ExportCompleted, store.updated["j-1"].Status())
	require.Equal(t, int64(3), store.updated["j-1"].RowCount())
	require.Len(t, blobs.put, 1)
	require.NotNil(t, store.updated["j-1"].ExpiresAt())
}

func TestExportWorkerEmptyQueue(t *testing.T) {
	w := worker.NewExportWorker(newFakeJobStore(), fakeExporter{}, newFakeBlobs(), fixedClock{t: time.Unix(1, 0).UTC()})
	processed, err := w.ProcessOne(context.Background())
	require.NoError(t, err)
	require.False(t, processed)
}

func TestExportWorkerExportFails(t *testing.T) {
	store := newFakeJobStore()
	store.pending = []domain.ExportJob{pendingJob(t, "j-2")}
	w := worker.NewExportWorker(store, fakeExporter{err: errors.New("es timeout")}, newFakeBlobs(), fixedClock{t: time.Unix(1, 0).UTC()})

	processed, err := w.ProcessOne(context.Background())
	require.True(t, processed)
	require.Error(t, err)
	require.Equal(t, domain.ExportFailed, store.updated["j-2"].Status())
}

// ---- report scheduler tests ----

type fakeScheduleStore struct {
	enabled []domain.ReportSchedule
	updated map[string]domain.ReportSchedule
}

func (s *fakeScheduleStore) Save(context.Context, domain.ReportSchedule) error { return nil }
func (s *fakeScheduleStore) ByID(context.Context, string, string) (domain.ReportSchedule, error) {
	return domain.ReportSchedule{}, domain.ErrReportScheduleNotFound
}
func (s *fakeScheduleStore) Update(_ context.Context, sch domain.ReportSchedule) error {
	s.updated[sch.ID()] = sch
	return nil
}
func (s *fakeScheduleStore) Delete(context.Context, string, string) error { return nil }
func (s *fakeScheduleStore) ListByWorkspace(context.Context, string) ([]domain.ReportSchedule, error) {
	return s.enabled, nil
}
func (s *fakeScheduleStore) ListEnabled(context.Context) ([]domain.ReportSchedule, error) {
	return s.enabled, nil
}

// fakeCron: Next = anchor + interval (đủ để test due logic).
type fakeCron struct{ interval time.Duration }

func (c fakeCron) Next(_, _ string, after time.Time) (time.Time, error) {
	return after.Add(c.interval), nil
}

type fakeGen struct{ data []byte }

func (f fakeGen) Generate(context.Context, domain.ReportSchedule) ([]byte, error) { return f.data, nil }

type fakeDelivery struct{ delivered int }

func (f *fakeDelivery) Deliver(context.Context, domain.ReportSchedule, []byte) error {
	f.delivered++
	return nil
}

func mkSchedule(t *testing.T, createdAt time.Time) domain.ReportSchedule {
	t.Helper()
	s, err := domain.NewReportSchedule(domain.NewScheduleInput{
		ID: "s-1", WorkspaceID: "ws-1", ReportType: domain.ReportSLOWeekly, CronExpr: "0 9 * * 1",
		Format: domain.FormatPDF, Recipients: []string{"a@b.c"}, Now: createdAt,
	})
	require.NoError(t, err)
	return s
}

func TestReportSchedulerRunsDue(t *testing.T) {
	created := time.Unix(0, 0).UTC()
	store := &fakeScheduleStore{enabled: []domain.ReportSchedule{mkSchedule(t, created)}, updated: map[string]domain.ReportSchedule{}}
	delivery := &fakeDelivery{}
	// interval 1h, anchor=created(0); now=2h → next(1h) <= now → due.
	sched := worker.NewReportScheduler(store, store, fakeCron{interval: time.Hour}, fakeGen{data: []byte("pdf")}, delivery, fixedClock{t: created.Add(2 * time.Hour)})

	res, err := sched.Sweep(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, res.Scanned)
	require.Equal(t, 1, res.Ran)
	require.Equal(t, 1, delivery.delivered)
	require.NotNil(t, store.updated["s-1"].LastRunAt())
}

func TestReportSchedulerSkipsNotDue(t *testing.T) {
	created := time.Unix(0, 0).UTC()
	store := &fakeScheduleStore{enabled: []domain.ReportSchedule{mkSchedule(t, created)}, updated: map[string]domain.ReportSchedule{}}
	delivery := &fakeDelivery{}
	// interval 10h, now=2h → next(10h) > now → không due.
	sched := worker.NewReportScheduler(store, store, fakeCron{interval: 10 * time.Hour}, fakeGen{}, delivery, fixedClock{t: created.Add(2 * time.Hour)})

	res, err := sched.Sweep(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0, res.Ran)
	require.Equal(t, 0, delivery.delivered)
}
