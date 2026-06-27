// Package sender cài đặt ports.Sender cho từng loại kênh: slack/teams/webhook/
// pagerduty (HTTP) và email (SMTP). Sender thuần I/O — payload builder tách riêng
// (pure) để test. Cấu hình lấy từ msg.Config (đã giải mã ở repository).
package sender

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/namdam97/logmon/backend/internal/notification/domain"
	"github.com/namdam97/logmon/backend/internal/notification/ports"
)

// _defaultTimeout giới hạn mỗi request gửi đi (chống treo worker).
const _defaultTimeout = 10 * time.Second

// _maxErrBody giới hạn số byte body lỗi đọc về (chống log phình).
const _maxErrBody = 512

// postJSON gửi POST JSON tới url, trả lỗi nếu status ngoài 2xx.
func postJSON(ctx context.Context, client *http.Client, url string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("post: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, _maxErrBody))
		return fmt.Errorf("status %d: %s", resp.StatusCode, bytes.TrimSpace(snippet))
	}
	return nil
}

func defaultClient() *http.Client { return &http.Client{Timeout: _defaultTimeout} }

// --- Slack ---

// SlackSender gửi qua incoming webhook (config: webhook_url).
type SlackSender struct{ client *http.Client }

var _ ports.Sender = (*SlackSender)(nil)

// NewSlackSender tạo sender Slack.
func NewSlackSender() *SlackSender { return &SlackSender{client: defaultClient()} }

// Send POST {"text": ...} tới webhook_url.
func (s *SlackSender) Send(ctx context.Context, msg domain.Message) error {
	url := msg.Config["webhook_url"]
	if url == "" {
		return fmt.Errorf("slack: missing webhook_url")
	}
	return postJSON(ctx, s.client, url, slackPayload(msg))
}

func slackPayload(msg domain.Message) map[string]string {
	return map[string]string{"text": "*" + msg.Subject + "*\n" + msg.Body}
}

// --- Teams ---

// TeamsSender gửi qua incoming webhook connector (config: webhook_url).
type TeamsSender struct{ client *http.Client }

var _ ports.Sender = (*TeamsSender)(nil)

// NewTeamsSender tạo sender Teams.
func NewTeamsSender() *TeamsSender { return &TeamsSender{client: defaultClient()} }

// Send POST text card tới webhook_url.
func (s *TeamsSender) Send(ctx context.Context, msg domain.Message) error {
	url := msg.Config["webhook_url"]
	if url == "" {
		return fmt.Errorf("teams: missing webhook_url")
	}
	return postJSON(ctx, s.client, url, teamsPayload(msg))
}

func teamsPayload(msg domain.Message) map[string]string {
	return map[string]string{"title": msg.Subject, "text": msg.Body}
}

// --- Webhook (generic) ---

// WebhookSender POST payload chuẩn tới endpoint tùy ý (config: url).
type WebhookSender struct{ client *http.Client }

var _ ports.Sender = (*WebhookSender)(nil)

// NewWebhookSender tạo sender webhook generic.
func NewWebhookSender() *WebhookSender { return &WebhookSender{client: defaultClient()} }

// Send POST {event_type, event_ref, subject, body} tới url.
func (s *WebhookSender) Send(ctx context.Context, msg domain.Message) error {
	url := msg.Config["url"]
	if url == "" {
		return fmt.Errorf("webhook: missing url")
	}
	return postJSON(ctx, s.client, url, webhookPayload(msg))
}

func webhookPayload(msg domain.Message) map[string]string {
	return map[string]string{
		"event_type": msg.EventType,
		"event_ref":  msg.EventRef,
		"subject":    msg.Subject,
		"body":       msg.Body,
	}
}

// --- PagerDuty (Events API v2) ---

// _pagerDutyURL là endpoint Events API v2 mặc định (override qua config: api_url).
const _pagerDutyURL = "https://events.pagerduty.com/v2/enqueue"

// PagerDutySender trigger event qua Events API v2 (config: integration_key).
type PagerDutySender struct{ client *http.Client }

var _ ports.Sender = (*PagerDutySender)(nil)

// NewPagerDutySender tạo sender PagerDuty.
func NewPagerDutySender() *PagerDutySender { return &PagerDutySender{client: defaultClient()} }

// Send POST event trigger; dùng DedupKey để gom trigger/resolve.
func (s *PagerDutySender) Send(ctx context.Context, msg domain.Message) error {
	key := msg.Config["integration_key"]
	if key == "" {
		return fmt.Errorf("pagerduty: missing integration_key")
	}
	url := msg.Config["api_url"]
	if url == "" {
		url = _pagerDutyURL
	}
	return postJSON(ctx, s.client, url, pagerDutyPayload(msg, key))
}

func pagerDutyPayload(msg domain.Message, key string) map[string]any {
	return map[string]any{
		"routing_key":  key,
		"event_action": "trigger",
		"dedup_key":    msg.DedupKey,
		"payload": map[string]string{
			"summary":  msg.Subject,
			"source":   "logmon",
			"severity": "critical",
		},
	}
}
