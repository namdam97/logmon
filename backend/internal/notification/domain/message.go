package domain

import "time"

// DeliveryStatus là trạng thái một lần gửi (ghi notification_history).
type DeliveryStatus string

// Các trạng thái delivery.
const (
	StatusSent     DeliveryStatus = "sent"
	StatusFailed   DeliveryStatus = "failed"
	StatusRetrying DeliveryStatus = "retrying"
)

// Message là một thông báo ĐÃ render, sẵn sàng để Sender gửi đi. Sender dùng
// Config (đã giải mã) + Subject/Body; DedupKey cho PagerDuty (trigger/resolve cùng key).
type Message struct {
	ChannelID   string
	WorkspaceID string
	ChannelType string
	Config      map[string]string
	EventType   string
	EventRef    string
	Subject     string
	Body        string
	DedupKey    string
	// Attempt là số lần đã thử gửi (0 = lần đầu) — worker dùng để backoff/giới hạn retry.
	Attempt int
}

// HistoryEntry là read model một bản ghi lịch sử gửi (UI debug "vì sao không nhận").
type HistoryEntry struct {
	WorkspaceID  string
	ChannelID    string
	EventType    string
	EventRef     string
	Status       DeliveryStatus
	ResponseCode int
	ErrorMessage string
	SentAt       time.Time
}
