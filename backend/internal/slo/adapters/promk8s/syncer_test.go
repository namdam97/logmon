package promk8s_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/namdam97/logmon/backend/internal/slo/adapters/promk8s"
	"github.com/namdam97/logmon/backend/internal/slo/domain"
)

type fakeReader struct {
	slos []domain.SLO
	err  error
}

func (r *fakeReader) ByID(context.Context, domain.SLOID) (domain.SLO, error) {
	return domain.SLO{}, domain.ErrSLONotFound
}
func (r *fakeReader) List(context.Context, string) ([]domain.SLO, error) { return r.slos, nil }
func (r *fakeReader) ListAll(context.Context) ([]domain.SLO, error)      { return r.slos, r.err }

type fakeStatus struct {
	synced, errored int
	lastErr         string
}

func (s *fakeStatus) MarkSynced(context.Context, time.Time) error { s.synced++; return nil }
func (s *fakeStatus) MarkSyncError(_ context.Context, msg string, _ time.Time) error {
	s.errored++
	s.lastErr = msg
	return nil
}

type fakeClock struct{}

func (fakeClock) Now() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }

type fakeApplier struct {
	got *unstructured.Unstructured
	err error
}

func (a *fakeApplier) Apply(_ context.Context, obj *unstructured.Unstructured) error {
	a.got = obj
	return a.err
}

func sloAvailability(t *testing.T) domain.SLO {
	t.Helper()
	id, err := domain.NewSLOID("slo-1")
	require.NoError(t, err)
	s, err := domain.NewSLO(domain.NewSLOInput{
		ID:          id,
		WorkspaceID: "ws",
		Name:        "checkout availability",
		Service:     "checkout",
		SLIType:     domain.SLIAvailability,
		Target:      0.999,
		WindowDays:  28,
		CreatedAt:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	return s
}

func TestSyncAppliesSLORules(t *testing.T) {
	reader := &fakeReader{slos: []domain.SLO{sloAvailability(t)}}
	status := &fakeStatus{}
	app := &fakeApplier{}
	s := promk8s.NewSyncer(reader, status, fakeClock{}, app, "logmon-slo", "logmon")

	require.NoError(t, s.Sync(context.Background()))

	require.Equal(t, 1, status.synced)
	require.NotNil(t, app.got)
	require.Equal(t, "PrometheusRule", app.got.GetKind())
	groups := app.got.Object["spec"].(map[string]any)["groups"].([]any)
	require.NotEmpty(t, groups, "mỗi SLO sinh ít nhất 1 group")
	rules := groups[0].(map[string]any)["rules"].([]any)
	require.NotEmpty(t, rules, "group có recording/alerting rules")
}

func TestSyncMarksErrorOnApplyFailure(t *testing.T) {
	reader := &fakeReader{slos: []domain.SLO{sloAvailability(t)}}
	status := &fakeStatus{}
	app := &fakeApplier{err: errors.New("boom")}
	s := promk8s.NewSyncer(reader, status, fakeClock{}, app, "n", "ns")

	err := s.Sync(context.Background())

	require.Error(t, err)
	require.Equal(t, 1, status.errored)
	require.Contains(t, status.lastErr, "boom")
}
