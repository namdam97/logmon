package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/alerting/domain"
)

var _now = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func validInput(t *testing.T) domain.NewAlertRuleInput {
	t.Helper()
	id, err := domain.NewRuleID("11111111-1111-1111-1111-111111111111")
	require.NoError(t, err)
	return domain.NewAlertRuleInput{
		ID:          id,
		WorkspaceID: "ws-default",
		Name:        "HighErrorRate",
		Expression:  `rate(logmon_http_requests_total{status=~"5.."}[5m]) > 0.05`,
		Service:     "logmon-api",
		ForDuration: 2 * time.Minute,
		Severity:    domain.SeverityCritical,
		Annotations: map[string]string{
			domain.AnnotationSummary:    "High 5xx on {{ $labels.service }}",
			domain.AnnotationRunbookURL: "https://wiki/runbooks/HighErrorRate",
		},
		CreatedAt: _now,
	}
}

func TestNewSeverity(t *testing.T) {
	tests := []struct {
		name    string
		give    string
		wantErr bool
	}{
		{name: "critical", give: "critical"},
		{name: "warning", give: "warning"},
		{name: "info", give: "info"},
		{name: "invalid", give: "fatal", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewSeverity(tt.give)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestSeverityMinForDuration(t *testing.T) {
	require.Equal(t, time.Minute, domain.SeverityCritical.MinForDuration())
	require.Equal(t, 5*time.Minute, domain.SeverityWarning.MinForDuration())
	require.Equal(t, time.Duration(0), domain.SeverityInfo.MinForDuration())
}

func TestNewAlertRuleValid(t *testing.T) {
	r, err := domain.NewAlertRule(validInput(t))
	require.NoError(t, err)
	require.Equal(t, "HighErrorRate", r.Name())
	require.Equal(t, "logmon-api", r.Service())
	require.True(t, r.IsEnabled())
	require.Equal(t, domain.SyncPending, r.SyncStatus())
	require.Equal(t, _now, r.CreatedAt())
}

func TestNewAlertRuleInvariants(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(in *domain.NewAlertRuleInput)
	}{
		{name: "empty name", mutate: func(in *domain.NewAlertRuleInput) { in.Name = "  " }},
		{name: "empty expression", mutate: func(in *domain.NewAlertRuleInput) { in.Expression = "" }},
		{name: "empty service", mutate: func(in *domain.NewAlertRuleInput) { in.Service = "" }},
		{name: "empty workspace", mutate: func(in *domain.NewAlertRuleInput) { in.WorkspaceID = "" }},
		{name: "missing summary", mutate: func(in *domain.NewAlertRuleInput) {
			in.Annotations = map[string]string{domain.AnnotationRunbookURL: "u"}
		}},
		{name: "missing runbook_url", mutate: func(in *domain.NewAlertRuleInput) {
			in.Annotations = map[string]string{domain.AnnotationSummary: "s"}
		}},
		{name: "for below critical minimum", mutate: func(in *domain.NewAlertRuleInput) {
			in.Severity = domain.SeverityCritical
			in.ForDuration = 30 * time.Second
		}},
		{name: "for below warning minimum", mutate: func(in *domain.NewAlertRuleInput) {
			in.Severity = domain.SeverityWarning
			in.ForDuration = time.Minute
		}},
		{name: "zero createdAt", mutate: func(in *domain.NewAlertRuleInput) { in.CreatedAt = time.Time{} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validInput(t)
			tt.mutate(&in)
			_, err := domain.NewAlertRule(in)
			require.Error(t, err)
			var ve *domain.ValidationError
			require.True(t, errors.As(err, &ve), "phải là ValidationError")
		})
	}
}

func TestAlertRuleTransitions(t *testing.T) {
	r, err := domain.NewAlertRule(validInput(t))
	require.NoError(t, err)
	later := _now.Add(time.Hour)

	disabled := r.Disabled(later)
	require.False(t, disabled.IsEnabled())
	require.Equal(t, domain.SyncPending, disabled.SyncStatus())
	require.True(t, r.IsEnabled(), "bản gốc bất biến")

	synced := r.MarkSynced(later)
	require.Equal(t, domain.SyncSynced, synced.SyncStatus())
	require.Empty(t, synced.SyncError())

	failed := r.MarkSyncError("promtool: parse error", later)
	require.Equal(t, domain.SyncError, failed.SyncStatus())
	require.Equal(t, "promtool: parse error", failed.SyncError())
	require.Equal(t, domain.SyncPending, r.SyncStatus(), "bản gốc bất biến")
}

func TestAlertRuleLabelsCopied(t *testing.T) {
	in := validInput(t)
	in.Labels = map[string]string{"team": "backend"}
	r, err := domain.NewAlertRule(in)
	require.NoError(t, err)
	// Sửa map gốc không ảnh hưởng aggregate (copy tại boundary).
	in.Labels["team"] = "hacked"
	require.Equal(t, "backend", r.Labels()["team"])
}
