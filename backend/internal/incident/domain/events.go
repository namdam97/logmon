package domain

// AggregateType định danh aggregate trong outbox.
const AggregateType = "Incident"

// Domain event types — phát qua outbox để notification gửi thông báo + (về sau)
// trigger escalation. Map sang notification event type ở composition root.
const (
	EventIncidentCreated  = "IncidentCreated"
	EventIncidentTriaged  = "IncidentTriaged"
	EventIncidentAssigned = "IncidentAssigned"
	EventIncidentResolved = "IncidentResolved"
	EventIncidentClosed   = "IncidentClosed"
)

// IncidentPayload là payload event incident — đủ để notification render template
// mà không cần đọc lại DB (khác SLO: tránh round-trip cho đường thông báo nóng).
type IncidentPayload struct {
	IncidentID  string `json:"incidentId"`
	WorkspaceID string `json:"workspaceId"`
	Title       string `json:"title"`
	Service     string `json:"service"`
	Severity    string `json:"severity"`
	Status      string `json:"status"`
}

// NewIncidentPayload dựng payload từ aggregate.
func NewIncidentPayload(i Incident) IncidentPayload {
	return IncidentPayload{
		IncidentID:  i.ID().String(),
		WorkspaceID: i.WorkspaceID(),
		Title:       i.Title(),
		Service:     i.Service(),
		Severity:    i.Severity().Label(),
		Status:      i.Status().String(),
	}
}
