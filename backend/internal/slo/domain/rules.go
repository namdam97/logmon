package domain

import (
	"fmt"
	"strconv"
	"strings"
)

// Rule generation cho SLO theo Multiwindow Multi-Burn-Rate (Google SRE Workbook
// Ch.5, ADR-025). Công thức là HẰNG SỐ cố định, đóng cứng table-driven dưới đây
// + golden-file test đối chiếu (điều kiện hội đồng GĐ3). KHÔNG thêm dependency:
// chỉ sinh cấu trúc rule thuần, adapter render YAML + validate rulefmt.

// Metric nguồn (khớp internal/shared/metrics) — service phân biệt qua target
// label `service` (thêm ở scrape config).
const (
	_metricRequests = "logmon_http_requests_total"
	_metricDuration = "logmon_http_request_duration_seconds"
	_recordPrefix   = "slo:errors:ratio_rate"
)

// recordingWindows là các cửa sổ rate cho recording rule (đủ cho 3 cặp MWMB).
var recordingWindows = []string{"5m", "30m", "1h", "6h", "3d"}

// burnPair là một cặp (long, short) window + burn-rate factor.
type burnPair struct {
	long   string
	short  string
	factor float64
}

// burnLevel gom các cặp burn cho một severity; nhiều cặp OR với nhau.
type burnLevel struct {
	severity string
	forDur   string
	pairs    []burnPair
}

// burnLevels: page (critical) = 1h/5m@14.4 OR 6h/30m@6; ticket (warning) =
// 3d/6h@1. Đúng bảng doc_v2/05 §4.2 / SRE Workbook.
var burnLevels = []burnLevel{
	{
		severity: "critical",
		forDur:   "2m",
		pairs: []burnPair{
			{long: "1h", short: "5m", factor: 14.4},
			{long: "6h", short: "30m", factor: 6},
		},
	},
	{
		severity: "warning",
		forDur:   "15m",
		pairs: []burnPair{
			{long: "3d", short: "6h", factor: 1},
		},
	},
}

// RecordingRule là một recording rule Prometheus (đã có sẵn label).
type RecordingRule struct {
	Record string
	Expr   string
	Labels map[string]string
}

// AlertingRule là một alerting rule Prometheus.
type AlertingRule struct {
	Alert       string
	Expr        string
	For         string
	Labels      map[string]string
	Annotations map[string]string
}

// RuleGroup gom toàn bộ rule sinh ra cho một SLO (1 group/SLO — tách biệt).
type RuleGroup struct {
	Name      string
	Recording []RecordingRule
	Alerting  []AlertingRule
}

// GenerateRuleGroup sinh recording + MWMB alerting rules cho SLO. Pure, deterministic.
func (s SLO) GenerateRuleGroup() RuleGroup {
	id := s.id.value
	baseLabels := func() map[string]string {
		return map[string]string{
			"slo":       id,
			"service":   s.service,
			"workspace": s.workspaceID,
		}
	}

	recording := make([]RecordingRule, 0, len(recordingWindows))
	for _, w := range recordingWindows {
		labels := baseLabels()
		labels["window"] = w
		recording = append(recording, RecordingRule{
			Record: _recordPrefix + w,
			Expr:   s.ErrorRatioQuery(w),
			Labels: labels,
		})
	}

	budget := strconv.FormatFloat(s.ErrorBudget(), 'g', -1, 64)
	alerting := make([]AlertingRule, 0, len(burnLevels))
	for _, lvl := range burnLevels {
		labels := baseLabels()
		labels["severity"] = lvl.severity
		alerting = append(alerting, AlertingRule{
			Alert:  "SLOBurnRate_" + id + "_" + lvl.severity,
			Expr:   burnExpr(id, budget, lvl.pairs),
			For:    lvl.forDur,
			Labels: labels,
			Annotations: map[string]string{
				"summary":     fmt.Sprintf("SLO %q (%s) burning error budget (%s)", s.name, s.service, lvl.severity),
				"runbook_url": "https://runbooks.logmon.local/slo-burn-rate",
			},
		})
	}

	return RuleGroup{
		Name:      "logmon-slo-" + id,
		Recording: recording,
		Alerting:  alerting,
	}
}

// ErrorRatioQuery trả về biểu thức PromQL tỉ lệ "bad" theo cửa sổ window — dùng
// chung cho recording rule VÀ budget snapshot job (DRY, cùng định nghĩa SLI).
//   - availability: 5xx / tổng request.
//   - latency: 1 − (request ≤ threshold / tổng) = tỉ lệ chậm hơn threshold.
func (s SLO) ErrorRatioQuery(window string) string {
	svc := s.service
	if s.sliType.IsLatency() {
		le := strconv.FormatFloat(float64(s.latencyMs)/1000, 'g', -1, 64)
		return fmt.Sprintf(
			`1 - (sum(rate(%s_bucket{service=%q,le=%q}[%s])) / sum(rate(%s_count{service=%q}[%s])))`,
			_metricDuration, svc, le, window, _metricDuration, svc, window,
		)
	}
	return fmt.Sprintf(
		`sum(rate(%s{service=%q,status=~"5.."}[%s])) / sum(rate(%s{service=%q}[%s]))`,
		_metricRequests, svc, window, _metricRequests, svc, window,
	)
}

// burnExpr build biểu thức MWMB: các cặp (long AND short vượt factor*budget) OR nhau.
func burnExpr(id, budget string, pairs []burnPair) string {
	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		factor := strconv.FormatFloat(p.factor, 'g', -1, 64)
		threshold := fmt.Sprintf("(%s * %s)", factor, budget)
		parts = append(parts, fmt.Sprintf(
			`(%s%s{slo=%q} > %s and %s%s{slo=%q} > %s)`,
			_recordPrefix, p.long, id, threshold,
			_recordPrefix, p.short, id, threshold,
		))
	}
	return strings.Join(parts, " or ")
}
