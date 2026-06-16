// server_test.go — Unit tests cho HTTP handlers và routing.
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// newTestServer tạo server với chaos tắt hoàn toàn để test deterministic.
func newTestServer() http.Handler {
	log := newLogger(nil, "disabled")
	mx := newMetrics()
	ch := newChaos(0, 0, func() float64 { return 0 })
	return buildServer(log, mx, ch)
}

func TestListOrders(t *testing.T) {
	srv := newTestServer()

	tests := []struct {
		give string // mô tả test case
		want int    // status code mong đợi
	}{
		{give: "GET /api/v1/orders trả 200", want: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.give, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil)
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, req)

			require.Equal(t, tt.want, rec.Code)

			var body map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			_, ok := body["orders"]
			require.True(t, ok, "response phải có key 'orders'")
		})
	}
}

func TestCreateOrder(t *testing.T) {
	tests := []struct {
		give     string
		body     string
		wantCode int
	}{
		{
			give:     "body hợp lệ → 201",
			body:     `{"item":"test-widget","quantity":3}`,
			wantCode: http.StatusCreated,
		},
		{
			give:     "thiếu item → 400",
			body:     `{"quantity":2}`,
			wantCode: http.StatusBadRequest,
		},
		{
			give:     "quantity = 0 → 400",
			body:     `{"item":"widget","quantity":0}`,
			wantCode: http.StatusBadRequest,
		},
		{
			give:     "quantity âm → 400",
			body:     `{"item":"widget","quantity":-1}`,
			wantCode: http.StatusBadRequest,
		},
		{
			give:     "body rỗng → 400",
			body:     `{}`,
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.give, func(t *testing.T) {
			srv := newTestServer()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/orders",
				strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, req)

			require.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

func TestCreateOrderResponseShape(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders",
		strings.NewReader(`{"item":"shape-check","quantity":5}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)

	var got order
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.NotEmpty(t, got.ID, "order mới phải có ID")
	require.Equal(t, "shape-check", got.Item)
	require.Equal(t, 5, got.Quantity)
}

func TestHealthz(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "ok", body["status"])
}

func TestMetricsEndpoint(t *testing.T) {
	srv := newTestServer()

	// Thực hiện 1 request trước để counter được ghi nhận.
	warmReq := httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil)
	warmRec := httptest.NewRecorder()
	srv.ServeHTTP(warmRec, warmReq)
	require.Equal(t, http.StatusOK, warmRec.Code)

	// Kiểm tra /metrics trả đúng metric names.
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	require.Contains(t, body, "logmon_http_requests_total")
	require.Contains(t, body, "logmon_http_request_duration_seconds")
	require.Contains(t, body, "logmon_http_requests_in_flight")
}
