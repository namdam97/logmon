package outbox

import "github.com/prometheus/client_golang/prometheus"

// Observer nhận tín hiệu metric từ relay. Tách interface để test dùng Nop.
type Observer interface {
	// ObserveLag set độ trễ outbox (giây) = tuổi event pending cũ nhất.
	ObserveLag(seconds float64)
	// IncFailed cộng số event chuyển sang trạng thái failed.
	IncFailed(n int)
}

// NopObserver bỏ qua mọi metric (mặc định cho relay/test).
type NopObserver struct{}

// ObserveLag là no-op.
func (NopObserver) ObserveLag(float64) {}

// IncFailed là no-op.
func (NopObserver) IncFailed(int) {}

// PrometheusObserver export logmon_outbox_lag_seconds + logmon_outbox_failed_total.
type PrometheusObserver struct {
	lag    prometheus.Gauge
	failed prometheus.Counter
}

var _ Observer = (*PrometheusObserver)(nil)

// NewPrometheusObserver đăng ký collector vào reg và trả về observer.
func NewPrometheusObserver(reg prometheus.Registerer) *PrometheusObserver {
	o := &PrometheusObserver{
		lag: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "logmon_outbox_lag_seconds",
			Help: "Tuổi (giây) của outbox event pending cũ nhất.",
		}),
		failed: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "logmon_outbox_failed_total",
			Help: "Tổng số outbox event chuyển sang trạng thái failed.",
		}),
	}
	reg.MustRegister(o.lag, o.failed)
	return o
}

// ObserveLag set gauge lag (giây).
func (o *PrometheusObserver) ObserveLag(seconds float64) { o.lag.Set(seconds) }

// IncFailed cộng counter failed.
func (o *PrometheusObserver) IncFailed(n int) { o.failed.Add(float64(n)) }
