// Package elasticsearch implement ports.DependencyReader: suy ra cạnh phụ thuộc
// service→service từ traces lưu trong ES (Jaeger v2 storage / OTel-native spans).
//
// Phương pháp: tổng hợp các client span (span.kind=client) mang attribute
// peer.service — mỗi client span là một lời gọi từ resource.service.name tới
// peer.service. Aggregation terms 2 cấp (source × target) + sub-filter span lỗi.
// Query DSL dựng bằng struct→JSON (KHÔNG concat chuỗi) nên giá trị workspace luôn
// là term value, không thể inject. Cách này cần OTel semconv attrs (doc_v2/12 §5.0).
package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/namdam97/logmon/backend/internal/topology/domain"
	"github.com/namdam97/logmon/backend/internal/topology/ports"
)

const (
	_httpTimeout = 10 * time.Second
	// _index là pattern index span Jaeger v2 / OTel-native traces.
	_index = "jaeger-span-*,traces-*"
	// _maxServices giới hạn số bucket mỗi cấp (chống cardinality bùng nổ).
	_maxServices = 200
)

// Reader gọi ES _search aggregation. baseURL ví dụ http://elasticsearch:9200.
type Reader struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
}

var _ ports.DependencyReader = (*Reader)(nil)

// NewReader tạo reader với timeout mặc định. baseURL không có dấu / cuối.
func NewReader(baseURL string) *Reader {
	return &Reader{baseURL: baseURL, httpClient: &http.Client{Timeout: _httpTimeout}}
}

// WithBasicAuth gắn basic auth (functional option-lite cho ES có bảo mật).
func (r *Reader) WithBasicAuth(username, password string) *Reader {
	r.username, r.password = username, password
	return r
}

// Dependencies truy vấn ES, tổng hợp client span thành cạnh phụ thuộc.
func (r *Reader) Dependencies(ctx context.Context, workspaceID string, since time.Time) ([]domain.Edge, error) {
	body, err := json.Marshal(buildQuery(workspaceID, since))
	if err != nil {
		return nil, fmt.Errorf("marshal query: %w", err)
	}

	url := r.baseURL + "/" + _index + "/_search"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if r.username != "" {
		req.SetBasicAuth(r.username, r.password)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("query elasticsearch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("elasticsearch search: unexpected status %d", resp.StatusCode)
	}

	var raw aggResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}
	return toEdges(raw), nil
}

func buildQuery(workspaceID string, since time.Time) map[string]any {
	filters := []map[string]any{
		{"term": map[string]any{"attributes.span.kind": "client"}},
		{"exists": map[string]any{"field": "attributes.peer.service"}},
		{"range": map[string]any{"@timestamp": map[string]any{"gte": since.UTC().Format(time.RFC3339Nano)}}},
	}
	if workspaceID != "" {
		filters = append(filters, map[string]any{
			"term": map[string]any{"resource.attributes.workspace_id": workspaceID},
		})
	}

	return map[string]any{
		"size":  0,
		"query": map[string]any{"bool": map[string]any{"filter": filters}},
		"aggs": map[string]any{
			"sources": map[string]any{
				"terms": map[string]any{"field": "resource.attributes.service.name", "size": _maxServices},
				"aggs": map[string]any{
					"targets": map[string]any{
						"terms": map[string]any{"field": "attributes.peer.service", "size": _maxServices},
						"aggs": map[string]any{
							"errors": map[string]any{
								"filter": map[string]any{"term": map[string]any{"status.code": "ERROR"}},
							},
						},
					},
				},
			},
		},
	}
}

// aggResponse ánh xạ phần aggregation cần dùng.
type aggResponse struct {
	Aggregations struct {
		Sources struct {
			Buckets []struct {
				Key     string `json:"key"`
				Targets struct {
					Buckets []struct {
						Key      string `json:"key"`
						DocCount int64  `json:"doc_count"`
						Errors   struct {
							DocCount int64 `json:"doc_count"`
						} `json:"errors"`
					} `json:"buckets"`
				} `json:"targets"`
			} `json:"buckets"`
		} `json:"sources"`
	} `json:"aggregations"`
}

func toEdges(raw aggResponse) []domain.Edge {
	edges := make([]domain.Edge, 0)
	for _, src := range raw.Aggregations.Sources.Buckets {
		for _, tgt := range src.Targets.Buckets {
			edges = append(edges, domain.Edge{
				Source:     src.Key,
				Target:     tgt.Key,
				CallCount:  tgt.DocCount,
				ErrorCount: tgt.Errors.DocCount,
			})
		}
	}
	return edges
}
