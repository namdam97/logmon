// Package http expose usage BC qua REST (doc_v2/07 §2.10 GET /usage). Tenant-scoped.
package http

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/namdam97/logmon/backend/internal/shared/auth"
	apperrors "github.com/namdam97/logmon/backend/internal/shared/errors"
	"github.com/namdam97/logmon/backend/internal/shared/httpx"
	"github.com/namdam97/logmon/backend/internal/usage/app"
	"github.com/namdam97/logmon/backend/internal/usage/domain"
)

const _timeLayout = "2006-01-02T15:04:05Z07:00"

// usageService là use case handler phụ thuộc (ISP).
type usageService interface {
	GetUsage(ctx context.Context, workspaceID string) (domain.UsageSummary, error)
	GetQuota(ctx context.Context, workspaceID string) (domain.Quota, error)
	SetQuota(ctx context.Context, in app.SetQuotaInput) (domain.Quota, error)
}

// Handler gắn usage use case vào HTTP routes.
type Handler struct {
	svc usageService
}

// NewHandler tạo handler.
func NewHandler(svc usageService) *Handler {
	return &Handler{svc: svc}
}

// Register gắn routes. authMW = tenantMW. Đọc=viewer, sửa quota=admin.
func (h *Handler) Register(rg *gin.RouterGroup, authMW gin.HandlerFunc) {
	rg.GET("/usage", authMW, h.usage)
	rg.GET("/usage/quota", authMW, h.getQuota)
	rg.PUT("/usage/quota", authMW, auth.RequireRole(auth.RoleAdmin), h.setQuota)
}

type usageResponse struct {
	IngestionBytes   int64   `json:"ingestionBytes"`
	StorageBytes     int64   `json:"storageBytes"`
	LogCount         int64   `json:"logCount"`
	EstimatedCostUSD float64 `json:"estimatedCostUsd"`
	PeriodStart      string  `json:"periodStart"`
	PeriodEnd        string  `json:"periodEnd"`
}

type quotaResponse struct {
	MaxIngestionBytesPerDay int64  `json:"maxIngestionBytesPerDay"`
	MaxStorageBytes         int64  `json:"maxStorageBytes"`
	RetentionDays           int    `json:"retentionDays"`
	UpdatedAt               string `json:"updatedAt"`
}

type setQuotaRequest struct {
	MaxIngestionBytesPerDay int64 `json:"maxIngestionBytesPerDay"`
	MaxStorageBytes         int64 `json:"maxStorageBytes"`
	RetentionDays           int   `json:"retentionDays"`
}

func toQuota(q domain.Quota) quotaResponse {
	return quotaResponse{
		MaxIngestionBytesPerDay: q.MaxIngestionBytesPerDay(),
		MaxStorageBytes:         q.MaxStorageBytes(),
		RetentionDays:           q.RetentionDays(),
		UpdatedAt:               q.UpdatedAt().Format(_timeLayout),
	}
}

func (h *Handler) usage(c *gin.Context) {
	u, err := h.svc.GetUsage(c.Request.Context(), h.wsID(c))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, usageResponse{
		IngestionBytes: u.IngestionBytes, StorageBytes: u.StorageBytes, LogCount: u.LogCount,
		EstimatedCostUSD: u.EstimatedCostUSD,
		PeriodStart:      u.PeriodStart.Format(_timeLayout), PeriodEnd: u.PeriodEnd.Format(_timeLayout),
	})
}

func (h *Handler) getQuota(c *gin.Context) {
	q, err := h.svc.GetQuota(c.Request.Context(), h.wsID(c))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, toQuota(q))
}

func (h *Handler) setQuota(c *gin.Context) {
	var req setQuotaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	q, err := h.svc.SetQuota(c.Request.Context(), app.SetQuotaInput{
		WorkspaceID: h.wsID(c), MaxIngestionBytesPerDay: req.MaxIngestionBytesPerDay,
		MaxStorageBytes: req.MaxStorageBytes, RetentionDays: req.RetentionDays,
	})
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, toQuota(q))
}

func (h *Handler) wsID(c *gin.Context) string {
	ws, _ := auth.WorkspaceIDFromContext(c)
	return ws
}

func fail(c *gin.Context, err error) {
	if ve, ok := apperrors.AsValidationError(err); ok {
		httpx.Fail(c, http.StatusBadRequest, ve.Error())
		return
	}
	httpx.Fail(c, http.StatusInternalServerError, "internal server error")
}
