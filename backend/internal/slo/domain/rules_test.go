package domain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/slo/domain"
)

func buildSLO(t *testing.T, in domain.NewSLOInput) domain.SLO {
	t.Helper()
	s, err := domain.NewSLO(in)
	require.NoError(t, err)
	return s
}

func TestGenerateRuleGroupAvailability(t *testing.T) {
	s := buildSLO(t, domain.NewSLOInput{
		ID:          mustID(),
		WorkspaceID: "ws-1",
		Name:        "checkout availability",
		Service:     "checkout",
		SLIType:     domain.SLIAvailability,
		Target:      0.999,
		WindowDays:  28,
		CreatedAt:   time.Date(2026, 6, 27, 0, 0, 0, 0, time.UTC),
	})

	g := s.GenerateRuleGroup()

	// Group name namespaced theo slo id (tránh collision với alerting + SLO khác).
	require.Contains(t, g.Name, "slo-1")

	// 5 recording rule window MWMB: 5m,30m,1h,6h,3d.
	require.Len(t, g.Recording, 5)
	rec := map[string]domain.RecordingRule{}
	for _, r := range g.Recording {
		rec[r.Record+"@"+r.Labels["window"]] = r
	}
	r5 := findRecording(t, g.Recording, "5m")
	require.Equal(t, "slo:errors:ratio_rate5m", r5.Record)
	// error ratio = 5xx / tổng, lọc theo service.
	require.Contains(t, r5.Expr, `logmon_http_requests_total{service="checkout",status=~"5.."}`)
	require.Contains(t, r5.Expr, `[5m]`)
	require.Equal(t, "slo-1", r5.Labels["slo"])
	require.Equal(t, "checkout", r5.Labels["service"])
	require.Equal(t, "ws-1", r5.Labels["workspace"])

	// 2 alerting rule: critical (page) + warning (ticket).
	require.Len(t, g.Alerting, 2)
	crit := findAlert(t, g.Alerting, "critical")
	// page = cặp 1h/5m@14.4 OR 6h/30m@6.
	require.Contains(t, crit.Expr, "14.4 * 0.001")
	require.Contains(t, crit.Expr, "6 * 0.001")
	require.Contains(t, crit.Expr, `slo:errors:ratio_rate1h{slo="slo-1"}`)
	require.Contains(t, crit.Expr, `slo:errors:ratio_rate5m{slo="slo-1"}`)
	require.Contains(t, crit.Expr, " or ")
	require.Contains(t, crit.Expr, " and ")
	require.NotEmpty(t, crit.Annotations["summary"])
	require.NotEmpty(t, crit.Annotations["runbook_url"])

	warn := findAlert(t, g.Alerting, "warning")
	require.Contains(t, warn.Expr, "1 * 0.001")
	require.Contains(t, warn.Expr, `slo:errors:ratio_rate3d{slo="slo-1"}`)
	require.NotContains(t, warn.Expr, " or ") // ticket chỉ 1 cặp
}

func TestGenerateRuleGroupLatency(t *testing.T) {
	s := buildSLO(t, domain.NewSLOInput{
		ID:                 mustID(),
		WorkspaceID:        "ws-1",
		Name:               "api latency",
		Service:            "api",
		SLIType:            domain.SLILatency,
		LatencyThresholdMs: 250,
		Target:             0.99,
		WindowDays:         28,
		CreatedAt:          time.Date(2026, 6, 27, 0, 0, 0, 0, time.UTC),
	})

	g := s.GenerateRuleGroup()

	r1h := findRecording(t, g.Recording, "1h")
	// latency error ratio = 1 - (request ≤ threshold / tổng); threshold 250ms = le="0.25".
	require.Contains(t, r1h.Expr, `logmon_http_request_duration_seconds_bucket{service="api",le="0.25"}`)
	require.Contains(t, r1h.Expr, "1 -")
	// budget = 1 - 0.99 = 0.01.
	crit := findAlert(t, g.Alerting, "critical")
	require.Contains(t, crit.Expr, "14.4 * 0.01")
}

func findRecording(t *testing.T, rules []domain.RecordingRule, window string) domain.RecordingRule {
	t.Helper()
	for _, r := range rules {
		if r.Labels["window"] == window {
			return r
		}
	}
	require.Failf(t, "recording rule not found", "window %s", window)
	return domain.RecordingRule{}
}

func findAlert(t *testing.T, rules []domain.AlertingRule, severity string) domain.AlertingRule {
	t.Helper()
	for _, r := range rules {
		if r.Labels["severity"] == severity {
			return r
		}
	}
	require.Failf(t, "alert rule not found", "severity %s", severity)
	return domain.AlertingRule{}
}

// đảm bảo expr không lẫn ký tự xuống dòng làm hỏng YAML một dòng.
func TestGenerateRuleGroupExprSingleLineable(t *testing.T) {
	s := buildSLO(t, giveValidInput())
	g := s.GenerateRuleGroup()
	for _, a := range g.Alerting {
		require.False(t, strings.Contains(a.Expr, "\n"), "alert expr must be single expression")
	}
}
