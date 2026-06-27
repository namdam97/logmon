// Package prometheus implement ports.MetricsQuerier qua HTTP query API của
// Prometheus/Thanos (instant query) — hand-rolled gọn nhẹ (như alertmanager
// client), không kéo client_golang/api.
package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/namdam97/logmon/backend/internal/slo/domain"
	"github.com/namdam97/logmon/backend/internal/slo/ports"
)

const _httpTimeout = 10 * time.Second

// Client gọi instant query API của Prometheus.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

var _ ports.MetricsQuerier = (*Client)(nil)

// NewClient tạo client với base URL Prometheus (vd http://prometheus:9090).
func NewClient(baseURL string) *Client {
	return &Client{baseURL: baseURL, httpClient: &http.Client{Timeout: _httpTimeout}}
}

// queryResponse khớp một phần response /api/v1/query.
type queryResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Value [2]any `json:"value"` // [timestamp float, value string]
		} `json:"result"`
	} `json:"data"`
	ErrorType string `json:"errorType"`
	Error     string `json:"error"`
}

// QueryScalar chạy instant query, trả về giá trị scalar đầu tiên. Query rỗng
// (không series) → domain.ErrNoData.
func (c *Client) QueryScalar(ctx context.Context, query string) (float64, error) {
	endpoint := c.baseURL + "/api/v1/query?query=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, fmt.Errorf("build query request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("query prometheus: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("prometheus query status %d", resp.StatusCode)
	}

	var qr queryResponse
	if err := json.NewDecoder(resp.Body).Decode(&qr); err != nil {
		return 0, fmt.Errorf("decode query response: %w", err)
	}
	if qr.Status != "success" {
		return 0, fmt.Errorf("prometheus query error: %s", qr.Error)
	}
	if len(qr.Data.Result) == 0 {
		return 0, domain.ErrNoData
	}

	raw, ok := qr.Data.Result[0].Value[1].(string)
	if !ok {
		return 0, fmt.Errorf("unexpected value type %T", qr.Data.Result[0].Value[1])
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("parse value %q: %w", raw, err)
	}
	return v, nil
}
