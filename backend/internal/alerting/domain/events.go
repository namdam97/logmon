package domain

// AggregateType định danh aggregate trong outbox.
const AggregateType = "AlertRule"

// Domain event types — phát qua outbox để kích hoạt rule sync pipeline.
const (
	EventAlertRuleCreated = "AlertRuleCreated"
	EventAlertRuleUpdated = "AlertRuleUpdated"
	EventAlertRuleDeleted = "AlertRuleDeleted"
)

// RulePayload là payload tối thiểu của event (syncer đọc chi tiết rule từ DB
// bằng ruleId — tránh nhồi toàn bộ state vào outbox).
type RulePayload struct {
	RuleID      string `json:"ruleId"`
	WorkspaceID string `json:"workspaceId"`
}
