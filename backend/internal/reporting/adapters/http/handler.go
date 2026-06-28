// Package http expose reporting BC qua REST (doc_v2/07 §2.10): scheduled reports
// + async export. Tenant-scoped (workspace + role từ context do RequireAuthWorkspace).
package http

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/namdam97/logmon/backend/internal/reporting/app/command"
	"github.com/namdam97/logmon/backend/internal/reporting/domain"
	"github.com/namdam97/logmon/backend/internal/shared/auth"
	apperrors "github.com/namdam97/logmon/backend/internal/shared/errors"
	"github.com/namdam97/logmon/backend/internal/shared/httpx"
)

const (
	_timeLayout   = "2006-01-02T15:04:05Z07:00"
	_signedURLTTL = 24 * time.Hour
)

// Use-case interfaces (ISP).
type scheduleCommands interface {
	Create(ctx context.Context, in command.CreateScheduleInput) (domain.ReportSchedule, error)
	SetEnabled(ctx context.Context, workspaceID, id string, enabled bool) (domain.ReportSchedule, error)
	Delete(ctx context.Context, workspaceID, id string) error
}
type exportCommands interface {
	Create(ctx context.Context, in command.CreateExportInput) (domain.ExportJob, error)
}
type reportQueries interface {
	ListSchedules(ctx context.Context, workspaceID string) ([]domain.ReportSchedule, error)
	GetExportJob(ctx context.Context, workspaceID, id string) (domain.ExportJob, error)
}
type urlSigner interface {
	SignedURL(ctx context.Context, key string, ttl time.Duration) (string, error)
}

// Handler gắn reporting use case vào HTTP routes.
type Handler struct {
	schedules scheduleCommands
	exports   exportCommands
	queries   reportQueries
	signer    urlSigner
}

// NewHandler tạo handler.
func NewHandler(schedules scheduleCommands, exports exportCommands, queries reportQueries, signer urlSigner) *Handler {
	return &Handler{schedules: schedules, exports: exports, queries: queries, signer: signer}
}

// Register gắn routes. authMW = tenantMW. Read=viewer, write schedule=admin, export=editor.
func (h *Handler) Register(rg *gin.RouterGroup, authMW gin.HandlerFunc) {
	admin := auth.RequireRole(auth.RoleAdmin)
	editor := auth.RequireRole(auth.RoleEditor)
	rg.GET("/reports/schedules", authMW, h.listSchedules)
	rg.POST("/reports/schedules", authMW, admin, h.createSchedule)
	rg.PUT("/reports/schedules/:id", authMW, admin, h.updateSchedule)
	rg.DELETE("/reports/schedules/:id", authMW, admin, h.deleteSchedule)
	rg.POST("/export/logs", authMW, editor, h.exportLogs)
	rg.POST("/export/metrics", authMW, editor, h.exportMetrics)
	rg.GET("/export/jobs/:id", authMW, h.getJob)
}

// ---- requests / responses ----

type createScheduleRequest struct {
	ReportType string   `json:"reportType"`
	CronExpr   string   `json:"cronExpression"`
	Timezone   string   `json:"timezone"`
	Format     string   `json:"format"`
	Recipients []string `json:"recipients"`
	ChannelID  string   `json:"channelId"`
}

type updateScheduleRequest struct {
	Enabled bool `json:"enabled"`
}

type exportRequest struct {
	Format      string         `json:"format"`
	QueryParams map[string]any `json:"queryParams"`
}

type scheduleResponse struct {
	ID         string   `json:"id"`
	ReportType string   `json:"reportType"`
	CronExpr   string   `json:"cronExpression"`
	Timezone   string   `json:"timezone"`
	Format     string   `json:"format"`
	Recipients []string `json:"recipients"`
	ChannelID  string   `json:"channelId,omitempty"`
	Enabled    bool     `json:"enabled"`
	LastRunAt  string   `json:"lastRunAt,omitempty"`
	CreatedAt  string   `json:"createdAt"`
}

type jobResponse struct {
	ID          string `json:"id"`
	ExportType  string `json:"exportType"`
	Format      string `json:"format"`
	Status      string `json:"status"`
	RowCount    int64  `json:"rowCount"`
	DownloadURL string `json:"downloadUrl,omitempty"`
	CreatedAt   string `json:"createdAt"`
	CompletedAt string `json:"completedAt,omitempty"`
	ExpiresAt   string `json:"expiresAt,omitempty"`
}

func toSchedule(s domain.ReportSchedule) scheduleResponse {
	resp := scheduleResponse{
		ID: s.ID(), ReportType: s.ReportType().String(), CronExpr: s.CronExpr(), Timezone: s.Timezone(),
		Format: s.Format().String(), Recipients: s.Recipients(), ChannelID: s.ChannelID(),
		Enabled: s.Enabled(), CreatedAt: s.CreatedAt().Format(_timeLayout),
	}
	if t := s.LastRunAt(); t != nil {
		resp.LastRunAt = t.Format(_timeLayout)
	}
	return resp
}

func (h *Handler) toJob(c *gin.Context, j domain.ExportJob) jobResponse {
	resp := jobResponse{
		ID: j.ID(), ExportType: j.ExportType().String(), Format: j.Format().String(),
		Status: j.Status().String(), RowCount: j.RowCount(), CreatedAt: j.CreatedAt().Format(_timeLayout),
	}
	if t := j.CompletedAt(); t != nil {
		resp.CompletedAt = t.Format(_timeLayout)
	}
	if t := j.ExpiresAt(); t != nil {
		resp.ExpiresAt = t.Format(_timeLayout)
	}
	// File sẵn sàng → cấp signed URL (best-effort; lỗi ký không chặn poll).
	if j.Status() == domain.ExportCompleted && j.FilePath() != "" {
		if url, err := h.signer.SignedURL(c.Request.Context(), j.FilePath(), _signedURLTTL); err == nil {
			resp.DownloadURL = url
		}
	}
	return resp
}

// ---- handlers ----

func (h *Handler) listSchedules(c *gin.Context) {
	list, err := h.queries.ListSchedules(c.Request.Context(), h.wsID(c))
	if err != nil {
		failDomain(c, err)
		return
	}
	out := make([]scheduleResponse, 0, len(list))
	for _, s := range list {
		out = append(out, toSchedule(s))
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *Handler) createSchedule(c *gin.Context) {
	var req createScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	s, err := h.schedules.Create(c.Request.Context(), command.CreateScheduleInput{
		WorkspaceID: h.wsID(c), ReportType: req.ReportType, CronExpr: req.CronExpr, Timezone: req.Timezone,
		Format: req.Format, Recipients: req.Recipients, ChannelID: req.ChannelID,
	})
	if err != nil {
		failDomain(c, err)
		return
	}
	httpx.OK(c, http.StatusCreated, toSchedule(s))
}

func (h *Handler) updateSchedule(c *gin.Context) {
	var req updateScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	s, err := h.schedules.SetEnabled(c.Request.Context(), h.wsID(c), c.Param("id"), req.Enabled)
	if err != nil {
		failDomain(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, toSchedule(s))
}

func (h *Handler) deleteSchedule(c *gin.Context) {
	if err := h.schedules.Delete(c.Request.Context(), h.wsID(c), c.Param("id")); err != nil {
		failDomain(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{"deleted": true})
}

func (h *Handler) exportLogs(c *gin.Context)    { h.export(c, domain.ExportLogs.String()) }
func (h *Handler) exportMetrics(c *gin.Context) { h.export(c, domain.ExportMetrics.String()) }

func (h *Handler) export(c *gin.Context, exportType string) {
	var req exportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	userID, _ := auth.UserIDFromContext(c)
	job, err := h.exports.Create(c.Request.Context(), command.CreateExportInput{
		WorkspaceID: h.wsID(c), UserID: userID, ExportType: exportType, Format: req.Format, QueryParams: req.QueryParams,
	})
	if err != nil {
		failDomain(c, err)
		return
	}
	// 202 Accepted: job đã nhận, poll GET /export/jobs/:id.
	httpx.OK(c, http.StatusAccepted, h.toJob(c, job))
}

func (h *Handler) getJob(c *gin.Context) {
	job, err := h.queries.GetExportJob(c.Request.Context(), h.wsID(c), c.Param("id"))
	if err != nil {
		failDomain(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, h.toJob(c, job))
}

func (h *Handler) wsID(c *gin.Context) string {
	ws, _ := auth.WorkspaceIDFromContext(c)
	return ws
}

// failDomain map domain error sang HTTP status (message generic).
func failDomain(c *gin.Context, err error) {
	switch {
	case isNotFound(err):
		httpx.Fail(c, http.StatusNotFound, "not found")
	default:
		if ve, ok := apperrors.AsValidationError(err); ok {
			httpx.Fail(c, http.StatusBadRequest, ve.Error())
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, "internal server error")
	}
}

func isNotFound(err error) bool {
	return errors.Is(err, domain.ErrReportScheduleNotFound) || errors.Is(err, domain.ErrExportJobNotFound)
}
