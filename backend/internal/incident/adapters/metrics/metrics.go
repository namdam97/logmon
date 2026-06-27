// Package metrics implement ports.Metrics của incident BC trên Prometheus
// client_golang. Naming theo convention: snake_case, prefix logmon_, Counter
// suffix _total; KHÔNG dùng high-cardinality label (severity 5 giá trị, service
// thấp). Histogram MTTA/MTTR không nhãn (doc_v2/06 §1.3).
package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/namdam97/logmon/backend/internal/incident/ports"
)

// _durationBuckets là bucket giây cho MTTA/MTTR: 1m → 24h.
var _durationBuckets = []float64{60, 300, 900, 1800, 3600, 7200, 21600, 43200, 86400}

// Collector gom các collector incident và implement ports.Metrics.
type Collector struct {
	total *prometheus.CounterVec
	open  *prometheus.GaugeVec
	mtta  prometheus.Histogram
	mttr  prometheus.Histogram
}

var _ ports.Metrics = (*Collector)(nil)

// New tạo Collector và đăng ký lên registerer (registry dùng chung của service).
func New(reg prometheus.Registerer) *Collector {
	c := &Collector{
		total: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "logmon_incidents_total",
			Help: "Total number of incidents created by severity and service.",
		}, []string{"severity", "service"}),
		open: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "logmon_incidents_open",
			Help: "Number of currently active (not resolved/closed) incidents by severity.",
		}, []string{"severity"}),
		mtta: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "logmon_incident_mtta_seconds",
			Help:    "Mean time to acknowledge (created to first assignment) in seconds.",
			Buckets: _durationBuckets,
		}),
		mttr: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "logmon_incident_mttr_seconds",
			Help:    "Mean time to resolve (created to resolved) in seconds.",
			Buckets: _durationBuckets,
		}),
	}
	reg.MustRegister(c.total, c.open, c.mtta, c.mttr)
	return c
}

// Opened ghi nhận incident vừa tạo: total++ và open gauge++.
func (c *Collector) Opened(severityLabel, service string) {
	c.total.WithLabelValues(severityLabel, service).Inc()
	c.open.WithLabelValues(severityLabel).Inc()
}

// Retriaged di chuyển open gauge khi severity đổi lúc còn active.
func (c *Collector) Retriaged(fromLabel, toLabel string) {
	c.open.WithLabelValues(fromLabel).Dec()
	c.open.WithLabelValues(toLabel).Inc()
}

// Assigned ghi nhận MTTA.
func (c *Collector) Assigned(mtta time.Duration) {
	c.mtta.Observe(mtta.Seconds())
}

// Resolved giảm open gauge và ghi nhận MTTR.
func (c *Collector) Resolved(severityLabel string, mttr time.Duration) {
	c.open.WithLabelValues(severityLabel).Dec()
	c.mttr.Observe(mttr.Seconds())
}

// NoMetrics là no-op ports.Metrics — dùng khi không cần đo (test/độc lập).
type NoMetrics struct{}

var _ ports.Metrics = NoMetrics{}

// Opened no-op.
func (NoMetrics) Opened(_, _ string) {}

// Retriaged no-op.
func (NoMetrics) Retriaged(_, _ string) {}

// Assigned no-op.
func (NoMetrics) Assigned(_ time.Duration) {}

// Resolved no-op.
func (NoMetrics) Resolved(_ string, _ time.Duration) {}
