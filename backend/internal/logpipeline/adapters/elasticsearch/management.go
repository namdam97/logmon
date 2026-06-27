package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/namdam97/logmon/backend/internal/logpipeline/domain"
	"github.com/namdam97/logmon/backend/internal/logpipeline/ports"
)

// Management gọi ES management APIs: ILM policy, cluster health, data stream stats.
// Tách khỏi Client (search) nhưng cùng baseURL/auth.
type Management struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
}

// Verify compliance tại compile time.
var (
	_ ports.ILMApplier       = (*Management)(nil)
	_ ports.PipelineHealth   = (*Management)(nil)
	_ ports.DataStreamReader = (*Management)(nil)
)

// NewManagement tạo adapter với basic auth + timeout mặc định.
func NewManagement(baseURL, username, password string) *Management {
	return &Management{
		baseURL:    baseURL,
		username:   username,
		password:   password,
		httpClient: &http.Client{Timeout: _httpTimeout},
	}
}

// Apply PUT _ilm/policy/logs-{namespace}-policy với hot rollover + warm + delete.
func (m *Management) Apply(ctx context.Context, namespace string, p domain.ILMPolicy) error {
	body := map[string]any{
		"policy": map[string]any{
			"phases": map[string]any{
				"hot": map[string]any{
					"min_age": "0ms",
					"actions": map[string]any{
						"rollover": map[string]any{"max_age": days(p.HotDays)},
					},
				},
				"warm":   map[string]any{"min_age": days(p.WarmDays), "actions": map[string]any{}},
				"delete": map[string]any{"min_age": days(p.DeleteDays), "actions": map[string]any{"delete": map[string]any{}}},
			},
		},
	}
	path := "/_ilm/policy/logs-" + namespace + "-policy"
	return m.do(ctx, http.MethodPut, path, body, nil)
}

// Check GET _cluster/health → ES up/down. Collector/Kafka chưa probe (unknown).
func (m *Management) Check(ctx context.Context) domain.HealthStatus {
	st := domain.HealthStatus{Elasticsearch: "down", Collector: "unknown", Kafka: "unknown"}
	if err := m.do(ctx, http.MethodGet, "/_cluster/health", nil, nil); err == nil {
		st.Elasticsearch = "up"
	}
	return st
}

// Stats GET _data_stream/logs-*-{namespace}/_stats → thống kê data stream.
func (m *Management) Stats(ctx context.Context, namespace string) ([]domain.DataStreamStat, error) {
	var resp struct {
		DataStreams []struct {
			DataStream       string `json:"data_stream"`
			BackingIndices   int    `json:"backing_indices"`
			StoreSizeBytes   int64  `json:"store_size_bytes"`
			MaximumTimestamp int64  `json:"maximum_timestamp"`
		} `json:"data_streams"`
	}
	path := "/_data_stream/logs-*-" + namespace + "/_stats?human=false"
	if err := m.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	out := make([]domain.DataStreamStat, 0, len(resp.DataStreams))
	for _, ds := range resp.DataStreams {
		out = append(out, domain.DataStreamStat{
			Name:           ds.DataStream,
			SizeBytes:      ds.StoreSizeBytes,
			BackingIndices: ds.BackingIndices,
		})
	}
	return out, nil
}

// do thực hiện request JSON (out=nil khi không cần decode response).
func (m *Management) do(ctx context.Context, method, path string, body, out any) error {
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, m.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if m.username != "" {
		req.SetBasicAuth(m.username, m.password)
	}
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("es request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("es status %s", strconv.Itoa(resp.StatusCode))
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

func days(n int) string { return strconv.Itoa(n) + "d" }
