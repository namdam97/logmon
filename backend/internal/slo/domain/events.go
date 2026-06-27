package domain

// AggregateType định danh aggregate trong outbox.
const AggregateType = "SLO"

// Domain event types — phát qua outbox để kích hoạt SLO rule sync pipeline và
// (BudgetExhausted) thông báo / tạo incident.
const (
	EventSLODefined      = "SLODefined"
	EventSLOUpdated      = "SLOUpdated"
	EventSLODeleted      = "SLODeleted"
	EventBudgetExhausted = "BudgetExhausted"
)

// SLOPayload là payload tối thiểu của event sync (syncer đọc chi tiết SLO từ DB
// bằng sloId — tránh nhồi toàn bộ state vào outbox).
type SLOPayload struct {
	SLOID       string `json:"sloId"`
	WorkspaceID string `json:"workspaceId"`
}

// BudgetExhaustedPayload phát khi budget còn lại < ngưỡng (10%) — nguồn cho
// notification + auto-create incident (doc_v2/05 §4.3, doc_v2/06 §1.1).
type BudgetExhaustedPayload struct {
	SLOID                  string  `json:"sloId"`
	WorkspaceID            string  `json:"workspaceId"`
	Service                string  `json:"service"`
	BudgetRemainingPercent float64 `json:"budgetRemainingPercent"`
}
