// Package http expose slo BC qua REST (Gin): CRUD SLO (trigger rule generation)
// + budget + compliance. Map domain error sang HTTP status an toàn (httpx).
package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/namdam97/logmon/backend/internal/shared/httpx"
	"github.com/namdam97/logmon/backend/internal/slo/app/command"
	"github.com/namdam97/logmon/backend/internal/slo/app/query"
	"github.com/namdam97/logmon/backend/internal/slo/domain"
)

// Use-case interfaces (ISP — handler accept interface).
type sloCreator interface {
	Handle(ctx context.Context, in command.CreateSLOInput) (domain.SLO, error)
}
type sloUpdater interface {
	Handle(ctx context.Context, in command.UpdateSLOInput) (domain.SLO, error)
}
type sloDeleter interface {
	Handle(ctx context.Context, workspaceID, id string) error
}
type sloQueries interface {
	Get(ctx context.Context, workspaceID, id string) (domain.SLO, error)
	List(ctx context.Context, workspaceID string) ([]domain.SLO, error)
	Budget(ctx context.Context, workspaceID, id string) (query.BudgetView, error)
	Compliance(ctx context.Context, workspaceID string) ([]query.BudgetView, error)
}

// Handler gắn use case slo vào HTTP routes.
type Handler struct {
	creator     sloCreator
	updater     sloUpdater
	deleter     sloDeleter
	queries     sloQueries
	workspaceID string
}

// NewHandler tạo Handler. workspaceID là workspace mặc định (multi-tenancy 3.6).
func NewHandler(creator sloCreator, updater sloUpdater, deleter sloDeleter, queries sloQueries, workspaceID string) *Handler {
	return &Handler{creator: creator, updater: updater, deleter: deleter, queries: queries, workspaceID: workspaceID}
}

// Register gắn routes. authMW bảo vệ mọi route. compliance đăng ký TRƯỚC :id.
func (h *Handler) Register(rg *gin.RouterGroup, authMW gin.HandlerFunc) {
	rg.POST("/slos", authMW, h.create)
	rg.GET("/slos", authMW, h.list)
	rg.GET("/slos/compliance", authMW, h.compliance)
	rg.GET("/slos/:id", authMW, h.get)
	rg.PUT("/slos/:id", authMW, h.update)
	rg.DELETE("/slos/:id", authMW, h.delete)
	rg.GET("/slos/:id/budget", authMW, h.budget)
}

type sloRequest struct {
	Name               string  `json:"name"`
	Service            string  `json:"service"`
	SLIType            string  `json:"sliType"`
	LatencyThresholdMs int     `json:"latencyThresholdMs"`
	Target             float64 `json:"target"`
	WindowDays         int     `json:"windowDays"`
}

type sloResponse struct {
	ID                 string  `json:"id"`
	WorkspaceID        string  `json:"workspaceId"`
	Name               string  `json:"name"`
	Service            string  `json:"service"`
	SLIType            string  `json:"sliType"`
	LatencyThresholdMs int     `json:"latencyThresholdMs,omitempty"`
	Target             float64 `json:"target"`
	WindowDays         int     `json:"windowDays"`
	SyncStatus         string  `json:"syncStatus"`
	CreatedAt          string  `json:"createdAt"`
	UpdatedAt          string  `json:"updatedAt"`
}

func toResponse(s domain.SLO) sloResponse {
	return sloResponse{
		ID:                 s.ID().String(),
		WorkspaceID:        s.WorkspaceID(),
		Name:               s.Name(),
		Service:            s.Service(),
		SLIType:            s.SLIType().String(),
		LatencyThresholdMs: s.LatencyThresholdMs(),
		Target:             s.Target(),
		WindowDays:         s.WindowDays(),
		SyncStatus:         string(s.SyncStatus()),
		CreatedAt:          s.CreatedAt().Format(timeLayout),
		UpdatedAt:          s.UpdatedAt().Format(timeLayout),
	}
}

const timeLayout = "2006-01-02T15:04:05Z07:00"

func (h *Handler) create(c *gin.Context) {
	var req sloRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	slo, err := h.creator.Handle(c.Request.Context(), command.CreateSLOInput{
		WorkspaceID:        h.workspaceID,
		Name:               req.Name,
		Service:            req.Service,
		SLIType:            req.SLIType,
		LatencyThresholdMs: req.LatencyThresholdMs,
		Target:             req.Target,
		WindowDays:         req.WindowDays,
	})
	if err != nil {
		failDomain(c, err)
		return
	}
	httpx.OK(c, http.StatusCreated, toResponse(slo))
}

func (h *Handler) list(c *gin.Context) {
	slos, err := h.queries.List(c.Request.Context(), h.workspaceID)
	if err != nil {
		failDomain(c, err)
		return
	}
	out := make([]sloResponse, 0, len(slos))
	for _, s := range slos {
		out = append(out, toResponse(s))
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *Handler) get(c *gin.Context) {
	slo, err := h.queries.Get(c.Request.Context(), h.workspaceID, c.Param("id"))
	if err != nil {
		failDomain(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, toResponse(slo))
}

func (h *Handler) update(c *gin.Context) {
	var req sloRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	slo, err := h.updater.Handle(c.Request.Context(), command.UpdateSLOInput{
		WorkspaceID:        h.workspaceID,
		ID:                 c.Param("id"),
		Name:               req.Name,
		Service:            req.Service,
		SLIType:            req.SLIType,
		LatencyThresholdMs: req.LatencyThresholdMs,
		Target:             req.Target,
		WindowDays:         req.WindowDays,
	})
	if err != nil {
		failDomain(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, toResponse(slo))
}

func (h *Handler) delete(c *gin.Context) {
	if err := h.deleter.Handle(c.Request.Context(), h.workspaceID, c.Param("id")); err != nil {
		failDomain(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

type budgetResponse struct {
	SLOID                  string  `json:"sloId"`
	HasData                bool    `json:"hasData"`
	CurrentSLI             float64 `json:"currentSli,omitempty"`
	BudgetTotalMinutes     float64 `json:"budgetTotalMinutes,omitempty"`
	BudgetRemainingMinutes float64 `json:"budgetRemainingMinutes,omitempty"`
	BudgetRemainingPercent float64 `json:"budgetRemainingPercent,omitempty"`
	BurnRate1h             float64 `json:"burnRate1h,omitempty"`
	BurnRate6h             float64 `json:"burnRate6h,omitempty"`
	BurnRate24h            float64 `json:"burnRate24h,omitempty"`
	RecordedAt             string  `json:"recordedAt,omitempty"`
}

func toBudget(v query.BudgetView) budgetResponse {
	resp := budgetResponse{SLOID: v.SLO.ID().String(), HasData: v.HasData}
	if !v.HasData {
		return resp
	}
	totalMinutes := v.SLO.ErrorBudget() * float64(v.SLO.WindowDays()) * 24 * 60
	resp.CurrentSLI = v.Snapshot.CurrentSLI()
	resp.BudgetTotalMinutes = totalMinutes
	resp.BudgetRemainingMinutes = totalMinutes * v.Snapshot.BudgetRemainingPercent() / 100
	resp.BudgetRemainingPercent = v.Snapshot.BudgetRemainingPercent()
	resp.BurnRate1h = v.Snapshot.BurnRate1h()
	resp.BurnRate6h = v.Snapshot.BurnRate6h()
	resp.BurnRate24h = v.Snapshot.BurnRate24h()
	resp.RecordedAt = v.Snapshot.RecordedAt().Format(timeLayout)
	return resp
}

func (h *Handler) budget(c *gin.Context) {
	v, err := h.queries.Budget(c.Request.Context(), h.workspaceID, c.Param("id"))
	if err != nil {
		failDomain(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, toBudget(v))
}

type complianceRow struct {
	SLO    sloResponse    `json:"slo"`
	Budget budgetResponse `json:"budget"`
}

func (h *Handler) compliance(c *gin.Context) {
	views, err := h.queries.Compliance(c.Request.Context(), h.workspaceID)
	if err != nil {
		failDomain(c, err)
		return
	}
	out := make([]complianceRow, 0, len(views))
	for _, v := range views {
		out = append(out, complianceRow{SLO: toResponse(v.SLO), Budget: toBudget(v)})
	}
	httpx.OK(c, http.StatusOK, out)
}

// failDomain map domain error sang HTTP status (message generic an toàn).
func failDomain(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrSLONotFound):
		httpx.Fail(c, http.StatusNotFound, "slo not found")
	case errors.Is(err, domain.ErrSLONameTaken):
		httpx.Fail(c, http.StatusConflict, "slo name already taken")
	default:
		var ve *domain.ValidationError
		if errors.As(err, &ve) {
			httpx.Fail(c, http.StatusBadRequest, ve.Error())
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, "internal server error")
	}
}
