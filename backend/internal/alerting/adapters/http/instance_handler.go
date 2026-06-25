package http

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/namdam97/logmon/backend/internal/alerting/app/command"
	"github.com/namdam97/logmon/backend/internal/alerting/domain"
	"github.com/namdam97/logmon/backend/internal/shared/httpx"
)

// webhookIngester là write-side use case nhận alert từ Alertmanager (ISP).
type webhookIngester interface {
	Handle(ctx context.Context, in command.IngestWebhookInput) (command.IngestResult, error)
}

// instanceReader là read-side use case liệt kê instance đang active (CQRS).
type instanceReader interface {
	ListActive(ctx context.Context, workspaceID string) ([]domain.AlertInstance, error)
}

// InstanceHandler expose webhook receiver (Alertmanager → LogMon) + read model
// alert đang active. Webhook bảo vệ bằng bearer token nội bộ; active dùng auth thường.
type InstanceHandler struct {
	ingester    webhookIngester
	reader      instanceReader
	workspaceID string
}

// NewInstanceHandler tạo handler. workspaceID là workspace mặc định GĐ2.
func NewInstanceHandler(ingester webhookIngester, reader instanceReader, workspaceID string) *InstanceHandler {
	return &InstanceHandler{ingester: ingester, reader: reader, workspaceID: workspaceID}
}

// Register gắn routes. bearerMW bảo vệ webhook (chỉ Alertmanager nội bộ gọi),
// authMW bảo vệ read model (người dùng đã đăng nhập).
func (h *InstanceHandler) Register(rg *gin.RouterGroup, authMW, bearerMW gin.HandlerFunc) {
	rg.POST("/alerts/webhook", bearerMW, h.webhook)
	rg.GET("/alerts/active", authMW, h.listActive)
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
		WorkspaceID: h.workspaceID,
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

func (h *InstanceHandler) listActive(c *gin.Context) {
	instances, err := h.reader.ListActive(c.Request.Context(), h.workspaceID)
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
	ID          string            `json:"id"`
	Fingerprint string            `json:"fingerprint"`
	Status      string            `json:"status"`
	FiredAt     string            `json:"firedAt"`
	ResolvedAt  string            `json:"resolvedAt,omitempty"`
	Labels      map[string]string `json:"labels"`
}

func toInstanceResponse(i domain.AlertInstance) instanceResponse {
	resolvedAt := ""
	if !i.ResolvedAt().IsZero() {
		resolvedAt = i.ResolvedAt().Format(time.RFC3339)
	}
	return instanceResponse{
		ID:          i.ID(),
		Fingerprint: i.Fingerprint().String(),
		Status:      string(i.Status()),
		FiredAt:     i.FiredAt().Format(time.RFC3339),
		ResolvedAt:  resolvedAt,
		Labels:      i.Labels(),
	}
}
