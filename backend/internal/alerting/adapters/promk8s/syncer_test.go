package promk8s_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/namdam97/logmon/backend/internal/alerting/adapters/promk8s"
	"github.com/namdam97/logmon/backend/internal/alerting/domain"
)

type fakeReader struct {
	rules []domain.AlertRule
	err   error
}

func (r *fakeReader) ByID(context.Context, domain.RuleID) (domain.AlertRule, error) {
	return domain.AlertRule{}, domain.ErrRuleNotFound
}
func (r *fakeReader) List(context.Context, string) ([]domain.AlertRule, error) { return r.rules, nil }
func (r *fakeReader) ListAll(context.Context) ([]domain.AlertRule, error)      { return r.rules, r.err }

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

func ruleWith(t *testing.T, name, expr string, enabled bool) domain.AlertRule {
	t.Helper()
	id, err := domain.NewRuleID("11111111-1111-1111-1111-111111111111")
	require.NoError(t, err)
	rule, err := domain.NewAlertRule(domain.NewAlertRuleInput{
		ID:          id,
		WorkspaceID: "ws",
		Name:        name,
		Expression:  expr,
		Service:     "logmon-api",
		ForDuration: 2 * time.Minute,
		Severity:    domain.SeverityCritical,
		Annotations: map[string]string{domain.AnnotationSummary: "s", domain.AnnotationRunbookURL: "u"},
		CreatedAt:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	if !enabled {
		rule = rule.Disabled(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	}
	return rule
}

func TestSyncAppliesPrometheusRule(t *testing.T) {
	reader := &fakeReader{rules: []domain.AlertRule{
		ruleWith(t, "HighErrorRate", "up == 0", true),
		ruleWith(t, "Disabled", "up == 1", false),
	}}
	status := &fakeStatus{}
	app := &fakeApplier{}
	s := promk8s.NewSyncer(reader, status, fakeClock{}, app, "logmon-alerting", "logmon")

	require.NoError(t, s.Sync(context.Background()))

	require.Equal(t, 1, status.synced)
	require.Equal(t, 0, status.errored)
	require.NotNil(t, app.got)
	require.Equal(t, "PrometheusRule", app.got.GetKind())
	require.Equal(t, "logmon-alerting", app.got.GetName())
	require.Equal(t, "logmon", app.got.GetNamespace())

	groups := app.got.Object["spec"].(map[string]any)["groups"].([]any)
	rules := groups[0].(map[string]any)["rules"].([]any)
	require.Len(t, rules, 1, "rule disabled phải bị loại")
	r := rules[0].(map[string]any)
	require.Equal(t, "HighErrorRate", r["alert"])
	require.Equal(t, "2m", r["for"])
	labels := r["labels"].(map[string]any)
	require.Equal(t, "critical", labels["severity"])
	require.Equal(t, "logmon-api", labels["service"])
}

func TestSyncMarksErrorOnApplyFailure(t *testing.T) {
	reader := &fakeReader{rules: []domain.AlertRule{ruleWith(t, "R", "up == 0", true)}}
	status := &fakeStatus{}
	app := &fakeApplier{err: errors.New("boom")}
	s := promk8s.NewSyncer(reader, status, fakeClock{}, app, "n", "ns")

	err := s.Sync(context.Background())

	require.Error(t, err)
	require.Equal(t, 1, status.errored)
	require.Equal(t, 0, status.synced)
	require.Contains(t, status.lastErr, "boom")
}
