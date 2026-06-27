package http

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/namdam97/logmon/backend/internal/incident/app/command"
	"github.com/namdam97/logmon/backend/internal/incident/domain"
	"github.com/namdam97/logmon/backend/internal/shared/httpx"
)

// Postmortem HTTP (doc_v2/06 §1.5): submit/xem postmortem + publish + action items.

// Use-case interfaces (ISP).
type postmortemSubmitter interface {
	Submit(ctx context.Context, in command.SubmitPostmortemInput) (domain.Postmortem, error)
	Publish(ctx context.Context, workspaceID, incidentID string) (domain.Postmortem, error)
	AddActionItem(ctx context.Context, in command.AddActionItemInput) (domain.ActionItem, error)
	UpdateActionItemStatus(ctx context.Context, workspaceID, incidentID, itemID, status string) (domain.ActionItem, error)
}
type postmortemQueries interface {
	GetByIncident(ctx context.Context, workspaceID, incidentID string) (domain.Postmortem, []domain.ActionItem, error)
}

// PostmortemHandler gắn use case postmortem vào HTTP routes (nest dưới incidents/:id).
type PostmortemHandler struct {
	handler     postmortemSubmitter
	queries     postmortemQueries
	workspaceID string
}

// NewPostmortemHandler tạo handler.
func NewPostmortemHandler(handler postmortemSubmitter, queries postmortemQueries, workspaceID string) *PostmortemHandler {
	return &PostmortemHandler{handler: handler, queries: queries, workspaceID: workspaceID}
}

// Register gắn routes. authMW bảo vệ mọi route.
func (h *PostmortemHandler) Register(rg *gin.RouterGroup, authMW gin.HandlerFunc) {
	rg.POST("/incidents/:id/postmortem", authMW, h.submit)
	rg.GET("/incidents/:id/postmortem", authMW, h.get)
	rg.POST("/incidents/:id/postmortem/publish", authMW, h.publish)
	rg.POST("/incidents/:id/postmortem/action-items", authMW, h.addActionItem)
	rg.PATCH("/incidents/:id/postmortem/action-items/:itemId", authMW, h.updateActionItem)
}

// ---- requests / responses ----

type impactRequest struct {
	DurationSeconds       int64   `json:"durationSeconds"`
	ErrorCount            int64   `json:"errorCount"`
	BudgetConsumedPercent float64 `json:"budgetConsumedPercent"`
	Summary               string  `json:"summary"`
}

type submitPostmortemRequest struct {
	RootCause       string        `json:"rootCause"`
	Impact          impactRequest `json:"impact"`
	TimelineSummary string        `json:"timelineSummary"`
	LessonsLearned  string        `json:"lessonsLearned"`
}

type addActionItemRequest struct {
	Title    string `json:"title"`
	Assignee string `json:"assignee"`
	DueDate  string `json:"dueDate"` // RFC3339, optional
}

type updateActionStatusRequest struct {
	Status string `json:"status"`
}

type impactResponse struct {
	DurationSeconds       int64   `json:"durationSeconds"`
	ErrorCount            int64   `json:"errorCount"`
	BudgetConsumedPercent float64 `json:"budgetConsumedPercent"`
	Summary               string  `json:"summary,omitempty"`
}

type postmortemResponse struct {
	ID              string         `json:"id"`
	IncidentID      string         `json:"incidentId"`
	Status          string         `json:"status"`
	RootCause       string         `json:"rootCause,omitempty"`
	Impact          impactResponse `json:"impact"`
	TimelineSummary string         `json:"timelineSummary,omitempty"`
	LessonsLearned  string         `json:"lessonsLearned,omitempty"`
	CreatedAt       string         `json:"createdAt"`
	UpdatedAt       string         `json:"updatedAt"`
	PublishedAt     string         `json:"publishedAt,omitempty"`
}

func toPostmortem(p domain.Postmortem) postmortemResponse {
	im := p.Impact()
	resp := postmortemResponse{
		ID:         p.ID().String(),
		IncidentID: p.IncidentID().String(),
		Status:     p.Status().String(),
		RootCause:  p.RootCause(),
		Impact: impactResponse{
			DurationSeconds:       im.DurationSeconds,
			ErrorCount:            im.ErrorCount,
			BudgetConsumedPercent: im.BudgetConsumedPercent,
			Summary:               im.Summary,
		},
		TimelineSummary: p.TimelineSummary(),
		LessonsLearned:  p.LessonsLearned(),
		CreatedAt:       p.CreatedAt().Format(timeLayout),
		UpdatedAt:       p.UpdatedAt().Format(timeLayout),
	}
	if t := p.PublishedAt(); t != nil {
		resp.PublishedAt = t.Format(timeLayout)
	}
	return resp
}

type actionItemResponse struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Assignee    string `json:"assignee,omitempty"`
	DueDate     string `json:"dueDate,omitempty"`
	Status      string `json:"status"`
	CreatedAt   string `json:"createdAt"`
	CompletedAt string `json:"completedAt,omitempty"`
}

func toActionItem(a domain.ActionItem) actionItemResponse {
	resp := actionItemResponse{
		ID:        a.ID(),
		Title:     a.Title(),
		Assignee:  a.Assignee(),
		Status:    a.Status().String(),
		CreatedAt: a.CreatedAt().Format(timeLayout),
	}
	if t := a.DueDate(); t != nil {
		resp.DueDate = t.Format(timeLayout)
	}
	if t := a.CompletedAt(); t != nil {
		resp.CompletedAt = t.Format(timeLayout)
	}
	return resp
}

type postmortemDetailResponse struct {
	Postmortem  postmortemResponse   `json:"postmortem"`
	ActionItems []actionItemResponse `json:"actionItems"`
}

// ---- handlers ----

func (h *PostmortemHandler) submit(c *gin.Context) {
	var req submitPostmortemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	pm, err := h.handler.Submit(c.Request.Context(), command.SubmitPostmortemInput{
		WorkspaceID: h.workspaceID,
		IncidentID:  c.Param("id"),
		RootCause:   req.RootCause,
		Impact: domain.Impact{
			DurationSeconds:       req.Impact.DurationSeconds,
			ErrorCount:            req.Impact.ErrorCount,
			BudgetConsumedPercent: req.Impact.BudgetConsumedPercent,
			Summary:               req.Impact.Summary,
		},
		TimelineSummary: req.TimelineSummary,
		LessonsLearned:  req.LessonsLearned,
	})
	if err != nil {
		failDomain(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, toPostmortem(pm))
}

func (h *PostmortemHandler) get(c *gin.Context) {
	pm, items, err := h.queries.GetByIncident(c.Request.Context(), h.workspaceID, c.Param("id"))
	if err != nil {
		failDomain(c, err)
		return
	}
	out := make([]actionItemResponse, 0, len(items))
	for _, it := range items {
		out = append(out, toActionItem(it))
	}
	httpx.OK(c, http.StatusOK, postmortemDetailResponse{Postmortem: toPostmortem(pm), ActionItems: out})
}

func (h *PostmortemHandler) publish(c *gin.Context) {
	pm, err := h.handler.Publish(c.Request.Context(), h.workspaceID, c.Param("id"))
	if err != nil {
		failDomain(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, toPostmortem(pm))
}

func (h *PostmortemHandler) addActionItem(c *gin.Context) {
	var req addActionItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	var due *time.Time
	if req.DueDate != "" {
		parsed, err := time.Parse(time.RFC3339, req.DueDate)
		if err != nil {
			httpx.Fail(c, http.StatusBadRequest, "invalid dueDate (want RFC3339)")
			return
		}
		utc := parsed.UTC()
		due = &utc
	}
	item, err := h.handler.AddActionItem(c.Request.Context(), command.AddActionItemInput{
		WorkspaceID: h.workspaceID,
		IncidentID:  c.Param("id"),
		Title:       req.Title,
		Assignee:    req.Assignee,
		DueDate:     due,
	})
	if err != nil {
		failDomain(c, err)
		return
	}
	httpx.OK(c, http.StatusCreated, toActionItem(item))
}

func (h *PostmortemHandler) updateActionItem(c *gin.Context) {
	var req updateActionStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	item, err := h.handler.UpdateActionItemStatus(c.Request.Context(), h.workspaceID, c.Param("id"), c.Param("itemId"), req.Status)
	if err != nil {
		failDomain(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, toActionItem(item))
}
