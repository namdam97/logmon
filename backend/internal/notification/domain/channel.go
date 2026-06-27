package domain

import (
	"regexp"
	"strings"
	"time"
)

const maxNameLength = 100

// ChannelType là value object cho loại kênh thông báo (doc_v2/06 §2.2).
type ChannelType struct {
	value string
}

// Các loại kênh hỗ trợ GĐ3.
var (
	ChannelSlack     = ChannelType{"slack"}
	ChannelEmail     = ChannelType{"email"}
	ChannelPagerDuty = ChannelType{"pagerduty"}
	ChannelTeams     = ChannelType{"teams"}
	ChannelWebhook   = ChannelType{"webhook"}
)

// requiredConfigKeys liệt kê key config bắt buộc (non-empty) cho mỗi loại kênh.
var requiredConfigKeys = map[string][]string{
	ChannelSlack.value:     {"webhook_url"},
	ChannelTeams.value:     {"webhook_url"},
	ChannelWebhook.value:   {"url"},
	ChannelPagerDuty.value: {"integration_key"},
	ChannelEmail.value:     {"smtp_host", "from", "to"},
}

// NewChannelType validate và bọc loại kênh.
func NewChannelType(raw string) (ChannelType, error) {
	if _, ok := requiredConfigKeys[raw]; ok {
		return ChannelType{raw}, nil
	}
	return ChannelType{}, newValidationError("channelType", "must be one of slack|email|pagerduty|teams|webhook")
}

// String trả về biểu diễn chuỗi.
func (t ChannelType) String() string { return t.value }

// ChannelID là value object định danh channel.
type ChannelID struct{ value string }

// NewChannelID validate định danh không rỗng.
func NewChannelID(raw string) (ChannelID, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return ChannelID{}, newValidationError("id", "must not be empty")
	}
	return ChannelID{value: v}, nil
}

// String trả về biểu diễn chuỗi.
func (id ChannelID) String() string { return id.value }

// _eventTypePattern: event type là identifier an toàn (lowercase snake) — chống
// ký tự lạ lọt vào lookup/template.
var _eventTypePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Channel là aggregate root của notification BC. Field không export để giữ bất
// biến — config chứa secret (mã hóa at-rest ở repository, plaintext trong RAM).
type Channel struct {
	id          ChannelID
	workspaceID string
	name        string
	channelType ChannelType
	config      map[string]string
	events      []string
	enabled     bool
	createdAt   time.Time
	updatedAt   time.Time
}

// NewChannelInput gom tham số tạo channel.
type NewChannelInput struct {
	ID          ChannelID
	WorkspaceID string
	Name        string
	ChannelType ChannelType
	Config      map[string]string
	Events      []string
	CreatedAt   time.Time
}

type channelFields struct {
	name        string
	workspaceID string
	channelType ChannelType
	config      map[string]string
	events      []string
}

func validateChannelFields(f channelFields) (name string, config map[string]string, events []string, err error) {
	name = strings.TrimSpace(f.name)
	switch {
	case name == "":
		return "", nil, nil, newValidationError("name", "must not be empty")
	case len(name) > maxNameLength:
		return "", nil, nil, newValidationError("name", "exceeds maximum length")
	}
	if strings.TrimSpace(f.workspaceID) == "" {
		return "", nil, nil, newValidationError("workspaceId", "must not be empty")
	}
	for _, key := range requiredConfigKeys[f.channelType.value] {
		if strings.TrimSpace(f.config[key]) == "" {
			return "", nil, nil, newValidationError("config."+key, "is required")
		}
	}
	if len(f.events) == 0 {
		return "", nil, nil, newValidationError("events", "must subscribe to at least one event type")
	}
	for _, e := range f.events {
		if !_eventTypePattern.MatchString(e) {
			return "", nil, nil, newValidationError("events", "invalid event type: "+e)
		}
	}
	return name, copyMap(f.config), copySlice(f.events), nil
}

// NewChannel tạo channel mới đã validate (mặc định enabled).
func NewChannel(in NewChannelInput) (Channel, error) {
	name, config, events, err := validateChannelFields(channelFields{
		name: in.Name, workspaceID: in.WorkspaceID, channelType: in.ChannelType,
		config: in.Config, events: in.Events,
	})
	if err != nil {
		return Channel{}, err
	}
	if in.CreatedAt.IsZero() {
		return Channel{}, newValidationError("createdAt", "must be set")
	}
	return Channel{
		id: in.ID, workspaceID: in.WorkspaceID, name: name, channelType: in.ChannelType,
		config: config, events: events, enabled: true,
		createdAt: in.CreatedAt, updatedAt: in.CreatedAt,
	}, nil
}

// UpdateInput gom field sửa được của channel.
type UpdateInput struct {
	Name        string
	ChannelType ChannelType
	Config      map[string]string
	Events      []string
	Enabled     bool
}

// Update trả bản copy đã đổi field (đã validate). Giữ id/workspaceId/createdAt.
func (c Channel) Update(in UpdateInput, now time.Time) (Channel, error) {
	name, config, events, err := validateChannelFields(channelFields{
		name: in.Name, workspaceID: c.workspaceID, channelType: in.ChannelType,
		config: in.Config, events: in.Events,
	})
	if err != nil {
		return Channel{}, err
	}
	n := c
	n.name = name
	n.channelType = in.ChannelType
	n.config = config
	n.events = events
	n.enabled = in.Enabled
	n.updatedAt = now
	return n, nil
}

// ReconstructInput dựng lại Channel từ DB đã hợp lệ.
type ReconstructInput struct {
	ID          ChannelID
	WorkspaceID string
	Name        string
	ChannelType ChannelType
	Config      map[string]string
	Events      []string
	Enabled     bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Reconstruct hydrate Channel từ persistence.
func Reconstruct(in ReconstructInput) Channel {
	return Channel{
		id: in.ID, workspaceID: in.WorkspaceID, name: in.Name, channelType: in.ChannelType,
		config: copyMap(in.Config), events: copySlice(in.Events), enabled: in.Enabled,
		createdAt: in.CreatedAt, updatedAt: in.UpdatedAt,
	}
}

// SubscribesTo cho biết channel có đăng ký nhận event type này không.
func (c Channel) SubscribesTo(eventType string) bool {
	for _, e := range c.events {
		if e == eventType {
			return true
		}
	}
	return false
}

// ID trả về định danh channel.
func (c Channel) ID() ChannelID { return c.id }

// WorkspaceID trả về workspace sở hữu channel.
func (c Channel) WorkspaceID() string { return c.workspaceID }

// Name trả về tên channel (duy nhất trong workspace).
func (c Channel) Name() string { return c.name }

// Type trả về loại kênh.
func (c Channel) Type() ChannelType { return c.channelType }

// Config trả về bản copy config (plaintext) — gồm secret, chỉ dùng nội bộ.
func (c Channel) Config() map[string]string { return copyMap(c.config) }

// ConfigValue trả về giá trị config theo key (rỗng nếu không có).
func (c Channel) ConfigValue(key string) string { return c.config[key] }

// Events trả về bản copy danh sách event type đã đăng ký.
func (c Channel) Events() []string { return copySlice(c.events) }

// IsEnabled cho biết channel có đang bật không.
func (c Channel) IsEnabled() bool { return c.enabled }

// CreatedAt trả về thời điểm tạo.
func (c Channel) CreatedAt() time.Time { return c.createdAt }

// UpdatedAt trả về thời điểm cập nhật gần nhất.
func (c Channel) UpdatedAt() time.Time { return c.updatedAt }

func copyMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func copySlice(s []string) []string {
	if len(s) == 0 {
		return []string{}
	}
	out := make([]string, len(s))
	copy(out, s)
	return out
}
