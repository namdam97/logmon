// metrics.go — Prometheus metrics cho demo-order service.
// Dùng registry riêng, KHÔNG default registry để tránh xung đột khi test.
package main

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// _httpBuckets là histogram buckets chuẩn theo spec (giây).
var _httpBuckets = []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}

// appMetrics gom các collector HTTP cùng registry sở hữu chúng.
type appMetrics struct {
	registry        *prometheus.Registry
	requestsTotal   *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
	inFlight        prometheus.Gauge
}

// newMetrics tạo appMetrics với registry riêng và đăng ký các collector.
func newMetrics() *appMetrics {
	reg := prometheus.NewRegistry()

	requestsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "logmon_http_requests_total",
			Help: "Tổng số HTTP requests phân theo method, path và status.",
		},
		[]string{"method", "path", "status"},
	)
	requestDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "logmon_http_request_duration_seconds",
			Help:    "Latency HTTP request (giây) phân theo method và path.",
			Buckets: _httpBuckets,
		},
		[]string{"method", "path"},
	)
	inFlight := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "logmon_http_requests_in_flight",
		Help: "Số HTTP requests đang xử lý.",
	})

	reg.MustRegister(requestsTotal, requestDuration, inFlight)

	return &appMetrics{
		registry:        reg,
		requestsTotal:   requestsTotal,
		requestDuration: requestDuration,
		inFlight:        inFlight,
	}
}

// Registry trả về Prometheus registry để gắn vào /metrics handler.
func (m *appMetrics) Registry() *prometheus.Registry {
	return m.registry
}

// ObserveRequest ghi nhận một request hoàn tất. path phải là route template
// (vd "/api/v1/orders") để tránh high cardinality.
func (m *appMetrics) ObserveRequest(method, path string, status int, dur time.Duration) {
	m.requestsTotal.WithLabelValues(method, path, strconv.Itoa(status)).Inc()
	m.requestDuration.WithLabelValues(method, path).Observe(dur.Seconds())
}

// InFlightInc tăng gauge in-flight khi request bắt đầu.
func (m *appMetrics) InFlightInc() { m.inFlight.Inc() }

// InFlightDec giảm gauge in-flight khi request kết thúc.
func (m *appMetrics) InFlightDec() { m.inFlight.Dec() }
