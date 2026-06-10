package metrics_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/shared/metrics"
)

func TestObserveRequestRecordsCounter(t *testing.T) {
	m := metrics.New()

	m.ObserveRequest(http.MethodGet, "/users/:id", http.StatusOK, 25*time.Millisecond)
	m.ObserveRequest(http.MethodGet, "/users/:id", http.StatusOK, 30*time.Millisecond)

	families, err := m.Registry().Gather()
	require.NoError(t, err)

	var total float64
	for _, mf := range families {
		if mf.GetName() == "logmon_http_requests_total" {
			for _, metric := range mf.GetMetric() {
				total += metric.GetCounter().GetValue()
			}
		}
	}
	require.Equal(t, float64(2), total)
}
