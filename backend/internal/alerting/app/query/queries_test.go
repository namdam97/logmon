package query_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/alerting/app/query"
	"github.com/namdam97/logmon/backend/internal/alerting/domain"
)

type fakeReader struct {
	byID map[string]domain.AlertRule
	list []domain.AlertRule
}

func (r *fakeReader) ByID(_ context.Context, id domain.RuleID) (domain.AlertRule, error) {
	rule, ok := r.byID[id.String()]
	if !ok {
		return domain.AlertRule{}, domain.ErrRuleNotFound
	}
	return rule, nil
}

func (r *fakeReader) List(_ context.Context, _ string) ([]domain.AlertRule, error) {
	return r.list, nil
}

func (r *fakeReader) ListAll(_ context.Context) ([]domain.AlertRule, error) {
	return r.list, nil
}

func sampleRule(t *testing.T, id string) domain.AlertRule {
	t.Helper()
	rid, err := domain.NewRuleID(id)
	require.NoError(t, err)
	rule, err := domain.NewAlertRule(domain.NewAlertRuleInput{
		ID:          rid,
		WorkspaceID: "ws-default",
		Name:        "R-" + id,
		Expression:  "up == 0",
		Service:     "svc",
		ForDuration: time.Minute,
		Severity:    domain.SeverityCritical,
		Annotations: map[string]string{domain.AnnotationSummary: "s", domain.AnnotationRunbookURL: "u"},
		CreatedAt:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	return rule
}

func TestRuleQueriesGet(t *testing.T) {
	rule := sampleRule(t, "abc")
	reader := &fakeReader{byID: map[string]domain.AlertRule{"abc": rule}}
	q := query.NewRuleQueries(reader)

	got, err := q.Get(context.Background(), "abc")
	require.NoError(t, err)
	require.Equal(t, "R-abc", got.Name())

	_, err = q.Get(context.Background(), "missing")
	require.ErrorIs(t, err, domain.ErrRuleNotFound)

	_, err = q.Get(context.Background(), "  ")
	require.Error(t, err, "id rỗng → validation error")
}

func TestRuleQueriesList(t *testing.T) {
	reader := &fakeReader{list: []domain.AlertRule{sampleRule(t, "a"), sampleRule(t, "b")}}
	q := query.NewRuleQueries(reader)

	rules, err := q.List(context.Background(), "ws-default")
	require.NoError(t, err)
	require.Len(t, rules, 2)
}
