// Package elasticsearch implement ports.LogSearcher: truy vấn data stream logs-*
// qua _search API. Query DSL dựng bằng struct→JSON (KHÔNG concat chuỗi) nên giá
// trị do người dùng nhập luôn là term/match value, không thể inject vào DSL.
package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/namdam97/logmon/backend/internal/logpipeline/domain"
	"github.com/namdam97/logmon/backend/internal/logpipeline/ports"
)

const (
	_httpTimeout = 10 * time.Second
	// _index là pattern data stream log (doc_v2/03) — đọc mọi logs-*.
	_index = "logs-*"
	// _serviceAttr là key resource attribute service.name trong doc OTel-native.
	_serviceAttr = "service.name"
)

// Client gọi Elasticsearch _search. baseURL ví dụ http://elasticsearch:9200.
type Client struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
}

var _ ports.LogSearcher = (*Client)(nil)

// NewClient tạo client với basic auth + timeout mặc định. baseURL không có dấu /
// cuối.
func NewClient(baseURL, username, password string) *Client {
	return &Client{
		baseURL:    baseURL,
		username:   username,
		password:   password,
		httpClient: &http.Client{Timeout: _httpTimeout},
	}
}

// Search dựng query DSL từ criteria và trả về read model.
func (c *Client) Search(ctx context.Context, crit domain.SearchCriteria) (domain.SearchResult, error) {
	body, err := json.Marshal(buildQuery(crit))
	if err != nil {
		return domain.SearchResult{}, fmt.Errorf("marshal query: %w", err)
	}

	url := c.baseURL + "/" + _index + "/_search"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return domain.SearchResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return domain.SearchResult{}, fmt.Errorf("query elasticsearch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return domain.SearchResult{}, fmt.Errorf("elasticsearch search: unexpected status %d", resp.StatusCode)
	}

	var raw searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return domain.SearchResult{}, fmt.Errorf("decode search response: %w", err)
	}
	return toResult(raw), nil
}

// buildQuery dựng map query DSL ES. Mỗi filter là một mệnh đề trong bool.filter
// (không tính điểm, chỉ lọc) trừ full-text dùng must.match trên body.text.
func buildQuery(c domain.SearchCriteria) map[string]any {
	filters := make([]map[string]any, 0, 5)
	if c.Service() != "" {
		filters = append(filters, term("resource.attributes."+_serviceAttr, c.Service()))
	}
	if c.Severity() != "" {
		filters = append(filters, term("severity_text", c.Severity()))
	}
	if c.TraceID() != "" {
		filters = append(filters, term("trace_id", c.TraceID()))
	}
	if c.HasFrom() || c.HasTo() {
		rng := map[string]any{}
		if c.HasFrom() {
			rng["gte"] = c.From().UTC().Format(time.RFC3339Nano)
		}
		if c.HasTo() {
			rng["lte"] = c.To().UTC().Format(time.RFC3339Nano)
		}
		filters = append(filters, map[string]any{"range": map[string]any{"@timestamp": rng}})
	}

	boolQuery := map[string]any{}
	if len(filters) > 0 {
		boolQuery["filter"] = filters
	}
	if c.Query() != "" {
		boolQuery["must"] = []map[string]any{
			{"match": map[string]any{"body.text": c.Query()}},
		}
	}

	query := map[string]any{"match_all": map[string]any{}}
	if len(boolQuery) > 0 {
		query = map[string]any{"bool": boolQuery}
	}

	return map[string]any{
		"size":             c.Limit(),
		"from":             c.Offset(),
		"track_total_hits": true,
		"query":            query,
		"sort": []map[string]any{
			{"@timestamp": map[string]any{"order": "desc"}},
		},
	}
}

func term(field, value string) map[string]any {
	return map[string]any{"term": map[string]any{field: value}}
}

// searchResponse ánh xạ phần ES _search cần dùng (OTel-native mapping).
type searchResponse struct {
	Hits struct {
		Total struct {
			Value int `json:"value"`
		} `json:"total"`
		Hits []struct {
			Source struct {
				Timestamp string `json:"@timestamp"`
				Severity  string `json:"severity_text"`
				Body      struct {
					Text string `json:"text"`
				} `json:"body"`
				TraceID  string `json:"trace_id"`
				SpanID   string `json:"span_id"`
				Resource struct {
					Attributes map[string]any `json:"attributes"`
				} `json:"resource"`
			} `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

func toResult(raw searchResponse) domain.SearchResult {
	entries := make([]domain.LogEntry, 0, len(raw.Hits.Hits))
	for _, h := range raw.Hits.Hits {
		ts, _ := time.Parse(time.RFC3339Nano, h.Source.Timestamp)
		entries = append(entries, domain.LogEntry{
			Timestamp: ts,
			Severity:  h.Source.Severity,
			Body:      h.Source.Body.Text,
			Service:   stringAttr(h.Source.Resource.Attributes[_serviceAttr]),
			TraceID:   h.Source.TraceID,
			SpanID:    h.Source.SpanID,
		})
	}
	return domain.SearchResult{Entries: entries, Total: raw.Hits.Total.Value}
}

func stringAttr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
