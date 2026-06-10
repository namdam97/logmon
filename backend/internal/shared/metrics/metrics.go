// Package metrics quản lý Prometheus registry và HTTP metrics. Naming theo
// convention: snake_case, prefix logmon_, Counter suffix _total. KHÔNG dùng
// high-cardinality labels (user_id, request_id, trace_id).
package metrics

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics gom các collector của HTTP layer cùng registry sở hữu chúng.
type Metrics struct {
	registry        *prometheus.Registry
	requestsTotal   *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
}

// New tạo Metrics với registry riêng và đăng ký các collector HTTP.
func New() *Metrics {
	reg := prometheus.NewRegistry()

	requestsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "logmon_http_requests_total",
			Help: "Total number of HTTP requests by method, path and status.",
		},
		[]string{"method", "path", "status"},
	)
	requestDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "logmon_http_request_duration_seconds",
			Help:    "HTTP request latency in seconds by method and path.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	reg.MustRegister(requestsTotal, requestDuration)

	return &Metrics{
		registry:        reg,
		requestsTotal:   requestsTotal,
		requestDuration: requestDuration,
	}
}

// Registry trả về Prometheus registry để gắn vào /metrics handler.
func (m *Metrics) Registry() *prometheus.Registry {
	return m.registry
}

// ObserveRequest ghi nhận một request đã hoàn tất. path nên là route template
// (vd "/users/:id") để tránh high cardinality.
func (m *Metrics) ObserveRequest(method, path string, status int, dur time.Duration) {
	m.requestsTotal.WithLabelValues(method, path, strconv.Itoa(status)).Inc()
	m.requestDuration.WithLabelValues(method, path).Observe(dur.Seconds())
}
