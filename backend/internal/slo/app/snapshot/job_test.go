package snapshot_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/slo/app/snapshot"
	"github.com/namdam97/logmon/backend/internal/slo/domain"
)

type fakeReader struct{ slos []domain.SLO }

func (r fakeReader) ByID(context.Context, domain.SLOID) (domain.SLO, error) {
	return domain.SLO{}, domain.ErrSLONotFound
}
func (r fakeReader) List(context.Context, string) ([]domain.SLO, error) { return r.slos, nil }
func (r fakeReader) ListAll(context.Context) ([]domain.SLO, error)      { return r.slos, nil }

// fakeQuerier trả ratio theo cửa sổ (so khớp substring trong query).
type fakeQuerier struct {
	byWindow map[string]float64
	noData   bool
}

func (q fakeQuerier) QueryScalar(_ context.Context, query string) (float64, error) {
	if q.noData {
		return 0, domain.ErrNoData
	}
	for w, v := range q.byWindow {
		if contains(query, "["+w+"]") {
			return v, nil
		}
	}
	return 0, nil
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (indexOf(s, sub) >= 0)
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

type fakeSnapshots struct {
	saved []domain.Snapshot
	prev  *domain.Snapshot
}

func (s *fakeSnapshots) Save(_ context.Context, snap domain.Snapshot) error {
	s.saved = append(s.saved, snap)
	return nil
}
func (s *fakeSnapshots) Latest(_ context.Context, _ domain.SLOID) (domain.Snapshot, error) {
	if s.prev == nil {
		return domain.Snapshot{}, domain.ErrSnapshotNotFound
	}
	return *s.prev, nil
}

type fakeTx struct{}

func (fakeTx) WithinTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type fakePublisher struct{ events []string }

func (p *fakePublisher) Publish(_ context.Context, _, _, eventType string, _ any) error {
	p.events = append(p.events, eventType)
	return nil
}

type fakeClock struct{ t time.Time }

func (c fakeClock) Now() time.Time { return c.t }

type nopLog struct{}

func (nopLog) Info(string, ...any)  {}
func (nopLog) Error(string, ...any) {}

func buildSLO(t *testing.T) domain.SLO {
	t.Helper()
	id, _ := domain.NewSLOID("slo-1")
	s, err := domain.NewSLO(domain.NewSLOInput{
		ID: id, WorkspaceID: "ws-1", Name: "checkout", Service: "checkout",
		SLIType: domain.SLIAvailability, Target: 0.999, WindowDays: 28,
		CreatedAt: time.Date(2026, 6, 27, 0, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	return s
}

func TestRunOnceSavesSnapshotAndEmitsExhausted(t *testing.T) {
	// budget = 0.001. window ratio = 0.00095 → consumed 95% → remaining 5% < 10%.
	q := fakeQuerier{byWindow: map[string]float64{
		"28d": 0.00095, "1h": 0.002, "6h": 0.001, "24h": 0.0005,
	}}
	snaps := &fakeSnapshots{} // no prev → coi như 100% trước đó → edge-trigger
	pub := &fakePublisher{}
	job := snapshot.NewJob(fakeReader{slos: []domain.SLO{buildSLO(t)}}, q, snaps, fakeTx{}, pub, fakeClock{time.Now()}, nopLog{})

	require.NoError(t, job.RunOnce(context.Background()))

	require.Len(t, snaps.saved, 1)
	got := snaps.saved[0]
	require.InDelta(t, 5.0, got.BudgetRemainingPercent(), 0.5)
	require.InDelta(t, 2.0, got.BurnRate1h(), 1e-9) // 0.002/0.001
	require.Equal(t, []string{domain.EventBudgetExhausted}, pub.events)
}

func TestRunOnceHealthyNoEvent(t *testing.T) {
	// window ratio rất nhỏ → budget gần 100% → không phát event.
	q := fakeQuerier{byWindow: map[string]float64{"28d": 0.00001, "1h": 0, "6h": 0, "24h": 0}}
	snaps := &fakeSnapshots{}
	pub := &fakePublisher{}
	job := snapshot.NewJob(fakeReader{slos: []domain.SLO{buildSLO(t)}}, q, snaps, fakeTx{}, pub, fakeClock{time.Now()}, nopLog{})

	require.NoError(t, job.RunOnce(context.Background()))

	require.Len(t, snaps.saved, 1)
	require.Empty(t, pub.events)
}

func TestRunOnceSkipsWhenNoData(t *testing.T) {
	job := snapshot.NewJob(fakeReader{slos: []domain.SLO{buildSLO(t)}}, fakeQuerier{noData: true}, &fakeSnapshots{}, fakeTx{}, &fakePublisher{}, fakeClock{time.Now()}, nopLog{})

	require.NoError(t, job.RunOnce(context.Background()))
}
