package http

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/namdam97/logmon/backend/internal/alerting/app/command"
	"github.com/namdam97/logmon/backend/internal/alerting/domain"
	"github.com/namdam97/logmon/backend/internal/shared/auth"
	"github.com/namdam97/logmon/backend/internal/shared/httpx"
)

// webhookIngester là write-side use case nhận alert từ Alertmanager (ISP).
type webhookIngester interface {
	Handle(ctx context.Context, in command.IngestWebhookInput) (command.IngestResult, error)
}

// instanceAcknowledger là write-side use case ack một instance (ISP).
type instanceAcknowledger interface {
	Handle(ctx context.Context, in command.AcknowledgeInput) (domain.AlertInstance, error)
}

// instanceReader là read-side use case liệt kê instance đang active (CQRS).
type instanceReader interface {
	ListActive(ctx context.Context, workspaceID string) ([]domain.AlertInstance, error)
}

// InstanceHandler expose webhook receiver (Alertmanager → LogMon), ack instance,
// và read model alert đang active. Webhook bảo vệ bằng bearer token nội bộ;
// ack/active dùng auth thường (người dùng đã đăng nhập).
type InstanceHandler struct {
	ingester     webhookIngester
	acknowledger instanceAcknowledger
	reader       instanceReader
	workspaceID  string
}

// NewInstanceHandler tạo handler. workspaceID là workspace mặc định GĐ2.
func NewInstanceHandler(ingester webhookIngester, acknowledger instanceAcknowledger, reader instanceReader, workspaceID string) *InstanceHandler {
	return &InstanceHandler{
		ingester:     ingester,
		acknowledger: acknowledger,
		reader:       reader,
		workspaceID:  workspaceID,
	}
}

// Register gắn routes. bearerMW bảo vệ webhook (chỉ Alertmanager nội bộ gọi),
// authMW bảo vệ ack + read model (người dùng đã đăng nhập).
func (h *InstanceHandler) Register(rg *gin.RouterGroup, authMW, bearerMW gin.HandlerFunc) {
	rg.POST("/alerts/webhook", bearerMW, h.webhook)
	rg.GET("/alerts/active", authMW, h.listActive)
	rg.POST("/alerts/:id/acknowledge", authMW, h.acknowledge)
}

// alertmanagerPayload là subset payload webhook v4 của Alertmanager mà ta dùng.
type alertmanagerPayload struct {
	Alerts []alertmanagerAlert `json:"alerts"`
}

type alertmanagerAlert struct {
	Status      string            `json:"status"`
	Fingerprint string            `json:"fingerprint"`
	StartsAt    time.Time         `json:"startsAt"`
	EndsAt      time.Time         `json:"endsAt"`
	Labels      map[string]string `json:"labels"`
}

func (h *InstanceHandler) webhook(c *gin.Context) {
	var payload alertmanagerPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid webhook payload")
		return
	}

	alerts := make([]command.WebhookAlert, 0, len(payload.Alerts))
	for _, a := range payload.Alerts {
		alerts = append(alerts, command.WebhookAlert{
			Status:      a.Status,
			Fingerprint: a.Fingerprint,
			StartsAt:    a.StartsAt,
			EndsAt:      a.EndsAt,
			Labels:      a.Labels,
		})
	}

	res, err := h.ingester.Handle(c.Request.Context(), command.IngestWebhookInput{
		WorkspaceID: h.wsID(c),
		Alerts:      alerts,
	})
	if err != nil {
		var ve *domain.ValidationError
		if errors.As(err, &ve) {
			httpx.Fail(c, http.StatusBadRequest, ve.Error())
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, "internal server error")
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{"firing": res.Firing, "resolved": res.Resolved})
}

func (h *InstanceHandler) acknowledge(c *gin.Context) {
	userID, ok := auth.UserIDFromContext(c)
	if !ok {
		httpx.Fail(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	inst, err := h.acknowledger.Handle(c.Request.Context(), command.AcknowledgeInput{
		WorkspaceID: h.wsID(c),
		InstanceID:  c.Param("id"),
		AckedBy:     userID,
	})
	if err != nil {
		failInstance(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, toInstanceResponse(inst))
}

// failInstance map domain error của instance sang HTTP status (message generic).
func failInstance(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrInstanceNotFound):
		httpx.Fail(c, http.StatusNotFound, "alert instance not found")
	case errors.Is(err, domain.ErrInstanceNotAcknowledgeable):
		httpx.Fail(c, http.StatusConflict, "alert instance is not firing")
	default:
		var ve *domain.ValidationError
		if errors.As(err, &ve) {
			httpx.Fail(c, http.StatusBadRequest, ve.Error())
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, "internal server error")
	}
}

func (h *InstanceHandler) listActive(c *gin.Context) {
	instances, err := h.reader.ListActive(c.Request.Context(), h.wsID(c))
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "internal server error")
		return
	}
	out := make([]instanceResponse, 0, len(instances))
	for _, inst := range instances {
		out = append(out, toInstanceResponse(inst))
	}
	httpx.OK(c, http.StatusOK, out)
}

type instanceResponse struct {
	ID             string            `json:"id"`
	Fingerprint    string            `json:"fingerprint"`
	Status         string            `json:"status"`
	FiredAt        string            `json:"firedAt"`
	AcknowledgedAt string            `json:"acknowledgedAt,omitempty"`
	AcknowledgedBy string            `json:"acknowledgedBy,omitempty"`
	ResolvedAt     string            `json:"resolvedAt,omitempty"`
	Labels         map[string]string `json:"labels"`
}

func toInstanceResponse(i domain.AlertInstance) instanceResponse {
	return instanceResponse{
		ID:             i.ID(),
		Fingerprint:    i.Fingerprint().String(),
		Status:         string(i.Status()),
		FiredAt:        i.FiredAt().Format(time.RFC3339),
		AcknowledgedAt: formatTime(i.AcknowledgedAt()),
		AcknowledgedBy: i.AcknowledgedBy(),
		ResolvedAt:     formatTime(i.ResolvedAt()),
		Labels:         i.Labels(),
	}
}

// formatTime trả về RFC3339, hoặc rỗng nếu zero (để omitempty bỏ field).
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

// wsID lấy workspace từ context (đã qua RequireAuthWorkspace); fallback sang
// workspace mặc định khi không có context (webhook machine-auth / test).
func (h *InstanceHandler) wsID(c *gin.Context) string {
	if ws, ok := auth.WorkspaceIDFromContext(c); ok {
		return ws
	}
	return h.workspaceID
}
