// Package alertmanager implement ports.SilenceGateway: proxy thao tác silence
// sang Alertmanager v2 API (/api/v2/silences). Alertmanager là source of truth —
// LogMon chỉ tạo/xoá/liệt kê, KHÔNG lưu silence và KHÔNG reimplement matching.
package alertmanager

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/namdam97/logmon/backend/internal/alerting/domain"
	"github.com/namdam97/logmon/backend/internal/alerting/ports"
)

const _httpTimeout = 5 * time.Second

// Client gọi Alertmanager v2 API. baseURL ví dụ http://alertmanager:9093.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

var _ ports.SilenceGateway = (*Client)(nil)

// NewClient tạo client với timeout mặc định. baseURL là base URL Alertmanager
// (không có dấu / cuối).
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: _httpTimeout},
	}
}

// matcherDTO ánh xạ matcher theo schema Alertmanager v2.
type matcherDTO struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	IsRegex bool   `json:"isRegex"`
	IsEqual bool   `json:"isEqual"`
}

// postableSilence là body POST /api/v2/silences (tạo mới — không có id).
type postableSilence struct {
	Matchers  []matcherDTO `json:"matchers"`
	StartsAt  time.Time    `json:"startsAt"`
	EndsAt    time.Time    `json:"endsAt"`
	CreatedBy string       `json:"createdBy"`
	Comment   string       `json:"comment"`
}

// Create POST silence mới, trả về silenceID Alertmanager sinh.
func (c *Client) Create(ctx context.Context, s domain.Silence) (string, error) {
	matchers := make([]matcherDTO, 0, len(s.Matchers()))
	for _, m := range s.Matchers() {
		matchers = append(matchers, matcherDTO{
			Name: m.Name(), Value: m.Value(), IsRegex: m.IsRegex(), IsEqual: m.IsEqual(),
		})
	}
	body, err := json.Marshal(postableSilence{
		Matchers:  matchers,
		StartsAt:  s.StartsAt(),
		EndsAt:    s.EndsAt(),
		CreatedBy: s.CreatedBy(),
		Comment:   s.Comment(),
	})
	if err != nil {
		return "", fmt.Errorf("marshal silence: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v2/silences", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("post silence: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("alertmanager create silence: unexpected status %d", resp.StatusCode)
	}

	var out struct {
		SilenceID string `json:"silenceID"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode silence response: %w", err)
	}
	return out.SilenceID, nil
}

// Delete huỷ silence. Alertmanager dùng path số ít /api/v2/silence/{id}.
func (c *Client) Delete(ctx context.Context, id string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/api/v2/silence/"+id, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete silence: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusNotFound:
		return domain.ErrSilenceNotFound
	default:
		return fmt.Errorf("alertmanager delete silence: unexpected status %d", resp.StatusCode)
	}
}

// gettableSilence là phần tử mảng GET /api/v2/silences.
type gettableSilence struct {
	ID        string                 `json:"id"`
	Status    struct{ State string } `json:"status"`
	Matchers  []matcherDTO           `json:"matchers"`
	StartsAt  time.Time              `json:"startsAt"`
	EndsAt    time.Time              `json:"endsAt"`
	CreatedBy string                 `json:"createdBy"`
	Comment   string                 `json:"comment"`
}

// List GET mọi silence, ánh xạ sang read model domain.SilenceView.
func (c *Client) List(ctx context.Context) ([]domain.SilenceView, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v2/silences", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list silences: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("alertmanager list silences: unexpected status %d", resp.StatusCode)
	}

	var raw []gettableSilence
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode silences: %w", err)
	}
	out := make([]domain.SilenceView, 0, len(raw))
	for _, g := range raw {
		matchers := make([]domain.SilenceMatcher, 0, len(g.Matchers))
		for _, m := range g.Matchers {
			matchers = append(matchers, domain.NewSilenceMatcher(m.Name, m.Value, m.IsRegex, m.IsEqual))
		}
		out = append(out, domain.SilenceView{
			ID:        g.ID,
			Status:    g.Status.State,
			Matchers:  matchers,
			StartsAt:  g.StartsAt,
			EndsAt:    g.EndsAt,
			CreatedBy: g.CreatedBy,
			Comment:   g.Comment,
		})
	}
	return out, nil
}
