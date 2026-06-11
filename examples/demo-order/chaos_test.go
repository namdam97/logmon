// chaos_test.go — Tests cho chaos injection logic.
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestChaos_ShouldError(t *testing.T) {
	tests := []struct {
		give      string
		errorRate float64
		randVal   float64
		want      bool
	}{
		{
			give:      "errorRate=1.0, rand=1.0 → false (boundary: 1.0 < 1.0 = false)",
			errorRate: 1.0,
			randVal:   1.0,
			want:      false,
		},
		{
			give:      "errorRate=0.99, rand=0.5 → true",
			errorRate: 0.99,
			randVal:   0.5,
			want:      true,
		},
		{
			give:      "errorRate=0.99, rand=0.999 → true",
			errorRate: 0.99,
			randVal:   0.999,
			want:      false,
		},
		{
			give:      "errorRate=0.0, rand=0.0 → false",
			errorRate: 0.0,
			randVal:   0.0,
			want:      false,
		},
		{
			give:      "errorRate=0.0, rand=0.5 → false",
			errorRate: 0.0,
			randVal:   0.5,
			want:      false,
		},
		{
			give:      "errorRate=1.0, rand=0.999 → true",
			errorRate: 1.0,
			randVal:   0.999,
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.give, func(t *testing.T) {
			ch := newChaos(tt.errorRate, 0, func() float64 { return tt.randVal })
			require.Equal(t, tt.want, ch.shouldError())
		})
	}
}

func TestChaos_ExtraDelay(t *testing.T) {
	tests := []struct {
		give           string
		extraLatencyMS int
		randVal        float64
		want           time.Duration
	}{
		{
			give:           "extraLatencyMS=0 → luôn 0",
			extraLatencyMS: 0,
			randVal:        0.9,
			want:           0,
		},
		{
			give:           "extraLatencyMS=100, rand=0.5 → 50ms",
			extraLatencyMS: 100,
			randVal:        0.5,
			want:           50 * time.Millisecond,
		},
		{
			give:           "extraLatencyMS=200, rand=0.0 → 0ms",
			extraLatencyMS: 200,
			randVal:        0.0,
			want:           0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.give, func(t *testing.T) {
			ch := newChaos(0, tt.extraLatencyMS, func() float64 { return tt.randVal })
			require.Equal(t, tt.want, ch.extraDelay())
		})
	}
}

// TestChaos_Integration_ErrorRate1 kiểm tra request /api/v1/orders trả 500
// khi chaos inject lỗi (errorRate=1.0, rand=0.999 < 1.0).
func TestChaos_Integration_ErrorRate1(t *testing.T) {
	log := newLogger(nil, "disabled")
	mx := newMetrics()
	// rand trả 0.999 < 1.0 → shouldError() = true
	ch := newChaos(1.0, 0, func() float64 { return 0.999 })
	srv := buildServer(log, mx, ch)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

// TestChaos_Integration_ErrorRate0 kiểm tra /api/v1/orders KHÔNG bao giờ 500
// khi errorRate=0.
func TestChaos_Integration_ErrorRate0(t *testing.T) {
	log := newLogger(nil, "disabled")
	mx := newMetrics()
	ch := newChaos(0.0, 0, nil)
	srv := buildServer(log, mx, ch)

	for range 10 {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, "errorRate=0 không được trả 500")
	}
}

// TestChaos_Integration_ExtraLatency kiểm tra middleware không abort khi chỉ
// có latency injection (không inject lỗi).
func TestChaos_Integration_ExtraLatency(t *testing.T) {
	log := newLogger(nil, "disabled")
	mx := newMetrics()
	// errorRate=0 nên không inject lỗi, extraLatencyMS=1 để cover branch delay.
	callCount := 0
	ch := newChaos(0.0, 1, func() float64 {
		callCount++
		return 0.5
	})
	srv := buildServer(log, mx, ch)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	// randFn được gọi ít nhất 2 lần: 1 cho extraDelay, 1 cho shouldError.
	require.GreaterOrEqual(t, callCount, 2)
}

// TestChaos_Healthz_NotAffected kiểm tra /healthz không bị chaos injection
// dù errorRate=1.0.
func TestChaos_Healthz_NotAffected(t *testing.T) {
	log := newLogger(nil, "disabled")
	mx := newMetrics()
	// rand luôn 0.999 < 1.0 → shouldError() = true cho /api/v1/*
	ch := newChaos(1.0, 0, func() float64 { return 0.999 })
	srv := buildServer(log, mx, ch)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "/healthz không bị chaos injection")
}
