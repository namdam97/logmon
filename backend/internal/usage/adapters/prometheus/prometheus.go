// Package prometheus implement ports.UsageReader bằng instant query lên Prometheus.
// Matcher workspace do backend inject (KHÔNG nhận PromQL thô từ client — chống
// label injection, pattern prom-label-proxy doc_v2/09 §3).
package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/namdam97/logmon/backend/internal/usage/ports"
)

const (
	_httpTimeout = 10 * time.Second
	// _window khớp cửa sổ usage tầng app (30 ngày).
	_window = "30d"
)

// Reader query Prometheus cho ingestion/log count/storage theo workspace.
type Reader struct {
	baseURL    string
	httpClient *http.Client
}

var _ ports.UsageReader = (*Reader)(nil)

// NewReader tạo reader (baseURL ví dụ http://prometheus:9090, không dấu / cuối).
func NewReader(baseURL string) *Reader {
	return &Reader{baseURL: baseURL, httpClient: &http.Client{Timeout: _httpTimeout}}
}

// IngestionBytes = sum(increase(logmon_ingested_bytes_total{workspace}[30d])).
func (r *Reader) IngestionBytes(ctx context.Context, workspaceID string, _ time.Time) (int64, error) {
	return r.scalar(ctx, fmt.Sprintf(`sum(increase(logmon_ingested_bytes_total{workspace=%q}[%s]))`, workspaceID, _window))
}

// LogCount = sum(increase(logmon_log_records_total{workspace}[30d])).
func (r *Reader) LogCount(ctx context.Context, workspaceID string, _ time.Time) (int64, error) {
	return r.scalar(ctx, fmt.Sprintf(`sum(increase(logmon_log_records_total{workspace=%q}[%s]))`, workspaceID, _window))
}

// StorageBytes = sum(logmon_storage_bytes{workspace}).
func (r *Reader) StorageBytes(ctx context.Context, workspaceID string) (int64, error) {
	return r.scalar(ctx, fmt.Sprintf(`sum(logmon_storage_bytes{workspace=%q})`, workspaceID))
}

// scalar chạy instant query, trả giá trị vector đầu tiên làm int64 (0 nếu rỗng).
func (r *Reader) scalar(ctx context.Context, promQL string) (int64, error) {
	u := r.baseURL + "/api/v1/query?query=" + url.QueryEscape(promQL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, fmt.Errorf("build query: %w", err)
	}
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("prometheus query: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= http.StatusBadRequest {
		return 0, fmt.Errorf("prometheus status %s", strconv.Itoa(resp.StatusCode))
	}
	var body struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Value [2]any `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0, fmt.Errorf("decode prometheus response: %w", err)
	}
	if body.Status != "success" || len(body.Data.Result) == 0 {
		return 0, nil
	}
	// value = [timestamp, "<number-as-string>"].
	s, ok := body.Data.Result[0].Value[1].(string)
	if !ok {
		return 0, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, nil
	}
	return int64(f), nil
}
