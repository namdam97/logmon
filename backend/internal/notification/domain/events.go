package domain

// AggregateType định danh aggregate trong outbox.
const AggregateType = "NotificationChannel"

// Notification event types (snake_case) — kênh đăng ký theo các giá trị này;
// send use case map domain event nghiệp vụ (AlertFired, BudgetExhausted,
// IncidentCreated...) sang event type tương ứng để render template (doc_v2/06 §2.3).
const (
	EventAlertFired        = "alert_fired"
	EventAlertResolved     = "alert_resolved"
	EventIncidentCreated   = "incident_created"
	EventIncidentResolved  = "incident_resolved"
	EventIncidentEscalated = "incident_escalated"
	EventSLOBudgetWarning  = "slo_budget_warning"
)
