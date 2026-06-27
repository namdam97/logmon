package http

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/namdam97/logmon/backend/internal/logpipeline/app/command"
	"github.com/namdam97/logmon/backend/internal/logpipeline/app/query"
	"github.com/namdam97/logmon/backend/internal/logpipeline/domain"
	"github.com/namdam97/logmon/backend/internal/shared/auth"
	apperrors "github.com/namdam97/logmon/backend/internal/shared/errors"
	"github.com/namdam97/logmon/backend/internal/shared/httpx"
)

// Use-case interfaces (ISP).
type pipelineCommands interface {
	SwitchMode(ctx context.Context, workspaceID, mode, by string) (domain.PipelineConfig, error)
	UpdateILM(ctx context.Context, workspaceID, namespace string, in command.UpdateILMInput, by string) (domain.PipelineConfig, error)
}
type dlqCommands interface {
	Retry(ctx context.Context, workspaceID string, ids []int64) (command.RetryResult, error)
}
type pipelineQueries interface {
	Status(ctx context.Context, workspaceID, namespace string) (query.StatusView, error)
	GetConfig(ctx context.Context, workspaceID string) (domain.PipelineConfig, error)
	ListDLQ(ctx context.Context, workspaceID, statusFilter string, limit int) ([]domain.DLQEntry, map[string]int, error)
	DataStreams(ctx context.Context, namespace string) ([]domain.DataStreamStat, error)
}

// PipelineHandler expose pipeline management API (doc_v2/07 §2.5). Mọi route
// tenant-scoped (workspace + role từ context do RequireAuthWorkspace gắn).
type PipelineHandler struct {
	cmds    pipelineCommands
	dlq     dlqCommands
	queries pipelineQueries
}

// NewPipelineHandler tạo handler.
func NewPipelineHandler(cmds pipelineCommands, dlq dlqCommands, queries pipelineQueries) *PipelineHandler {
	return &PipelineHandler{cmds: cmds, dlq: dlq, queries: queries}
}

// Register gắn routes. authMW = tenantMW (membership). Route ghi (mode/ILM/retry)
// yêu cầu admin (doc_v2/07: pipeline mode/ILM = admin).
func (h *PipelineHandler) Register(rg *gin.RouterGroup, authMW gin.HandlerFunc) {
	admin := auth.RequireRole(auth.RoleAdmin)
	rg.GET("/pipeline/status", authMW, h.status)
	rg.POST("/pipeline/mode", authMW, admin, h.switchMode)
	rg.GET("/pipeline/dlq", authMW, h.listDLQ)
	rg.POST("/pipeline/dlq/retry", authMW, admin, h.retryDLQ)
	rg.GET("/pipeline/ilm", authMW, h.getILM)
	rg.PUT("/pipeline/ilm", authMW, admin, h.updateILM)
	rg.GET("/pipeline/datastreams", authMW, h.dataStreams)
}

// ---- requests / responses ----

type switchModeRequest struct {
	Mode string `json:"mode"`
}

type ilmRequest struct {
	HotDays    int `json:"hotDays"`
	WarmDays   int `json:"warmDays"`
	DeleteDays int `json:"deleteDays"`
}

type retryRequest struct {
	IDs []int64 `json:"ids"`
}

type ilmResponse struct {
	HotDays    int `json:"hotDays"`
	WarmDays   int `json:"warmDays"`
	DeleteDays int `json:"deleteDays"`
}

type configResponse struct {
	Mode      string      `json:"mode"`
	ILM       ilmResponse `json:"ilm"`
	UpdatedAt string      `json:"updatedAt"`
	UpdatedBy string      `json:"updatedBy,omitempty"`
}

type statusResponse struct {
	Mode   string `json:"mode"`
	Health struct {
		Elasticsearch string `json:"elasticsearch"`
		Collector     string `json:"collector"`
		Kafka         string `json:"kafka"`
	} `json:"health"`
	DataStreams int    `json:"dataStreams"`
	UpdatedAt   string `json:"updatedAt"`
}

type dlqEntryResponse struct {
	ID            int64  `json:"id"`
	RawMessage    string `json:"rawMessage"`
	ErrorReason   string `json:"errorReason"`
	SourceService string `json:"sourceService,omitempty"`
	RetryCount    int    `json:"retryCount"`
	Status        string `json:"status"`
	CreatedAt     string `json:"createdAt"`
	RetriedAt     string `json:"retriedAt,omitempty"`
}

type dlqListResponse struct {
	Entries []dlqEntryResponse `json:"entries"`
	Counts  map[string]int     `json:"counts"`
}

type dataStreamResponse struct {
	Name           string `json:"name"`
	SizeBytes      int64  `json:"sizeBytes"`
	DocCount       int64  `json:"docCount"`
	BackingIndices int    `json:"backingIndices"`
}

const _timeLayout = "2006-01-02T15:04:05Z07:00"

func toConfig(c domain.PipelineConfig) configResponse {
	ilm := c.ILM()
	resp := configResponse{
		Mode:      c.Mode().String(),
		ILM:       ilmResponse{HotDays: ilm.HotDays, WarmDays: ilm.WarmDays, DeleteDays: ilm.DeleteDays},
		UpdatedAt: c.UpdatedAt().Format(_timeLayout),
		UpdatedBy: c.UpdatedBy(),
	}
	return resp
}

func toDLQEntry(e domain.DLQEntry) dlqEntryResponse {
	resp := dlqEntryResponse{
		ID:            e.ID(),
		RawMessage:    e.RawMessage(),
		ErrorReason:   e.ErrorReason(),
		SourceService: e.SourceService(),
		RetryCount:    e.RetryCount(),
		Status:        e.Status().String(),
		CreatedAt:     e.CreatedAt().Format(_timeLayout),
	}
	if t := e.RetriedAt(); t != nil {
		resp.RetriedAt = t.Format(_timeLayout)
	}
	return resp
}

// ---- handlers ----

func (h *PipelineHandler) status(c *gin.Context) {
	ws := h.wsID(c)
	view, err := h.queries.Status(c.Request.Context(), ws, ws)
	if err != nil {
		failPipeline(c, err)
		return
	}
	resp := statusResponse{Mode: view.Mode, DataStreams: view.DataStreams, UpdatedAt: view.UpdatedAt.Format(_timeLayout)}
	resp.Health.Elasticsearch = view.Health.Elasticsearch
	resp.Health.Collector = view.Health.Collector
	resp.Health.Kafka = view.Health.Kafka
	httpx.OK(c, http.StatusOK, resp)
}

func (h *PipelineHandler) switchMode(c *gin.Context) {
	var req switchModeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	cfg, err := h.cmds.SwitchMode(c.Request.Context(), h.wsID(c), req.Mode, actorFrom(c))
	if err != nil {
		failPipeline(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, toConfig(cfg))
}

func (h *PipelineHandler) getILM(c *gin.Context) {
	cfg, err := h.queries.GetConfig(c.Request.Context(), h.wsID(c))
	if err != nil {
		failPipeline(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, toConfig(cfg))
}

func (h *PipelineHandler) updateILM(c *gin.Context) {
	var req ilmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	ws := h.wsID(c)
	cfg, err := h.cmds.UpdateILM(c.Request.Context(), ws, ws,
		command.UpdateILMInput{HotDays: req.HotDays, WarmDays: req.WarmDays, DeleteDays: req.DeleteDays}, actorFrom(c))
	if err != nil {
		failPipeline(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, toConfig(cfg))
}

func (h *PipelineHandler) listDLQ(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	entries, counts, err := h.queries.ListDLQ(c.Request.Context(), h.wsID(c), c.Query("status"), limit)
	if err != nil {
		failPipeline(c, err)
		return
	}
	out := make([]dlqEntryResponse, 0, len(entries))
	for _, e := range entries {
		out = append(out, toDLQEntry(e))
	}
	httpx.OK(c, http.StatusOK, dlqListResponse{Entries: out, Counts: counts})
}

func (h *PipelineHandler) retryDLQ(c *gin.Context) {
	var req retryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.IDs) == 0 {
		httpx.Fail(c, http.StatusBadRequest, "ids required")
		return
	}
	res, err := h.dlq.Retry(c.Request.Context(), h.wsID(c), req.IDs)
	if err != nil {
		failPipeline(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{"retried": res.Retried, "failed": res.Failed})
}

func (h *PipelineHandler) dataStreams(c *gin.Context) {
	stats, err := h.queries.DataStreams(c.Request.Context(), h.wsID(c))
	if err != nil {
		failPipeline(c, err)
		return
	}
	out := make([]dataStreamResponse, 0, len(stats))
	for _, s := range stats {
		out = append(out, dataStreamResponse{Name: s.Name, SizeBytes: s.SizeBytes, DocCount: s.DocCount, BackingIndices: s.BackingIndices})
	}
	httpx.OK(c, http.StatusOK, out)
}

// wsID lấy workspace từ context (RequireAuthWorkspace đã gắn).
func (h *PipelineHandler) wsID(c *gin.Context) string {
	ws, _ := auth.WorkspaceIDFromContext(c)
	return ws
}

func actorFrom(c *gin.Context) string {
	if id, ok := auth.UserIDFromContext(c); ok {
		return id
	}
	return "system"
}

// failPipeline map lỗi sang HTTP: ValidationError→400, ES/upstream→502, còn lại→500.
func failPipeline(c *gin.Context, err error) {
	if ve, ok := apperrors.AsValidationError(err); ok {
		httpx.Fail(c, http.StatusBadRequest, ve.Error())
		return
	}
	httpx.Fail(c, http.StatusBadGateway, "pipeline backend unavailable")
}
