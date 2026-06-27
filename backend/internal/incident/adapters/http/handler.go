// Package http expose incident BC qua REST (Gin): CRUD + transition state machine
// + timeline. Map domain error sang HTTP status an toàn (httpx). Actor lấy từ JWT
// (UserIDFromContext) để ghi timeline audit.
package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/namdam97/logmon/backend/internal/incident/app/command"
	"github.com/namdam97/logmon/backend/internal/incident/domain"
	"github.com/namdam97/logmon/backend/internal/shared/auth"
	"github.com/namdam97/logmon/backend/internal/shared/httpx"
)

const timeLayout = "2006-01-02T15:04:05Z07:00"

// Use-case interfaces (ISP — handler accept interface).
type incidentCreator interface {
	Handle(ctx context.Context, in command.CreateIncidentInput) (domain.Incident, error)
}
type incidentTransitions interface {
	Triage(ctx context.Context, workspaceID, id, severity, description, actor string) (domain.Incident, error)
	Assign(ctx context.Context, workspaceID, id, assignee, actor string) (domain.Incident, error)
	StartMitigation(ctx context.Context, workspaceID, id, actor string) (domain.Incident, error)
	Resolve(ctx context.Context, workspaceID, id, note, actor string) (domain.Incident, error)
	RequirePostmortem(ctx context.Context, workspaceID, id, actor string) (domain.Incident, error)
	Close(ctx context.Context, workspaceID, id, note, actor string) (domain.Incident, error)
}
type incidentQueries interface {
	Get(ctx context.Context, workspaceID, id string) (domain.Incident, error)
	List(ctx context.Context, workspaceID string) ([]domain.Incident, error)
	ListActive(ctx context.Context, workspaceID string) ([]domain.Incident, error)
	Timeline(ctx context.Context, workspaceID, id string) ([]domain.TimelineEntry, error)
}

// Handler gắn use case incident vào HTTP routes.
type Handler struct {
	creator     incidentCreator
	transitions incidentTransitions
	queries     incidentQueries
	workspaceID string
}

// NewHandler tạo Handler. workspaceID là workspace mặc định (multi-tenancy 3.6).
func NewHandler(creator incidentCreator, transitions incidentTransitions, queries incidentQueries, workspaceID string) *Handler {
	return &Handler{creator: creator, transitions: transitions, queries: queries, workspaceID: workspaceID}
}

// Register gắn routes. authMW bảo vệ mọi route.
func (h *Handler) Register(rg *gin.RouterGroup, authMW gin.HandlerFunc) {
	rg.POST("/incidents", authMW, h.create)
	rg.GET("/incidents", authMW, h.list)
	rg.GET("/incidents/:id", authMW, h.get)
	rg.GET("/incidents/:id/timeline", authMW, h.timeline)
	rg.POST("/incidents/:id/triage", authMW, h.triage)
	rg.POST("/incidents/:id/assign", authMW, h.assign)
	rg.POST("/incidents/:id/mitigate", authMW, h.mitigate)
	rg.POST("/incidents/:id/resolve", authMW, h.resolve)
	rg.POST("/incidents/:id/require-postmortem", authMW, h.postmortem)
	rg.POST("/incidents/:id/close", authMW, h.close)
}

func actorFrom(c *gin.Context) string {
	if id, ok := auth.UserIDFromContext(c); ok {
		return id
	}
	return "system"
}

// ---- requests / responses ----

type createRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Service     string `json:"service"`
	Severity    string `json:"severity"`
}

type triageRequest struct {
	Severity    string `json:"severity"`
	Description string `json:"description"`
}

type assignRequest struct {
	Assignee string `json:"assignee"`
}

type noteRequest struct {
	Note string `json:"note"`
}

type incidentResponse struct {
	ID          string  `json:"id"`
	WorkspaceID string  `json:"workspaceId"`
	Title       string  `json:"title"`
	Description string  `json:"description,omitempty"`
	Service     string  `json:"service"`
	Severity    string  `json:"severity,omitempty"`
	Status      string  `json:"status"`
	Source      string  `json:"source"`
	SourceRef   string  `json:"sourceRef,omitempty"`
	Assignee    string  `json:"assignee,omitempty"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   string  `json:"updatedAt"`
	AssignedAt  string  `json:"assignedAt,omitempty"`
	ResolvedAt  string  `json:"resolvedAt,omitempty"`
	ClosedAt    string  `json:"closedAt,omitempty"`
	MTTASeconds float64 `json:"mttaSeconds,omitempty"`
	MTTRSeconds float64 `json:"mttrSeconds,omitempty"`
}

func toResponse(i domain.Incident) incidentResponse {
	resp := incidentResponse{
		ID:          i.ID().String(),
		WorkspaceID: i.WorkspaceID(),
		Title:       i.Title(),
		Description: i.Description(),
		Service:     i.Service(),
		Severity:    i.Severity().String(),
		Status:      i.Status().String(),
		Source:      i.Source().String(),
		SourceRef:   i.SourceRef(),
		Assignee:    i.Assignee(),
		CreatedAt:   i.CreatedAt().Format(timeLayout),
		UpdatedAt:   i.UpdatedAt().Format(timeLayout),
	}
	if t := i.AssignedAt(); t != nil {
		resp.AssignedAt = t.Format(timeLayout)
	}
	if t := i.ResolvedAt(); t != nil {
		resp.ResolvedAt = t.Format(timeLayout)
	}
	if t := i.ClosedAt(); t != nil {
		resp.ClosedAt = t.Format(timeLayout)
	}
	if d, ok := i.MTTA(); ok {
		resp.MTTASeconds = d.Seconds()
	}
	if d, ok := i.MTTR(); ok {
		resp.MTTRSeconds = d.Seconds()
	}
	return resp
}

type timelineResponse struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	FromStatus string `json:"fromStatus,omitempty"`
	ToStatus   string `json:"toStatus,omitempty"`
	Actor      string `json:"actor,omitempty"`
	Note       string `json:"note,omitempty"`
	At         string `json:"at"`
}

func toTimeline(e domain.TimelineEntry) timelineResponse {
	return timelineResponse{
		ID:         e.ID(),
		Kind:       string(e.Kind()),
		FromStatus: e.FromStatus().String(),
		ToStatus:   e.ToStatus().String(),
		Actor:      e.Actor(),
		Note:       e.Note(),
		At:         e.At().Format(timeLayout),
	}
}

// ---- handlers ----

func (h *Handler) create(c *gin.Context) {
	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	inc, err := h.creator.Handle(c.Request.Context(), command.CreateIncidentInput{
		WorkspaceID: h.wsID(c),
		Title:       req.Title,
		Description: req.Description,
		Service:     req.Service,
		Severity:    req.Severity,
		Source:      domain.SourceManual.String(),
		Actor:       actorFrom(c),
	})
	if err != nil {
		failDomain(c, err)
		return
	}
	httpx.OK(c, http.StatusCreated, toResponse(inc))
}

func (h *Handler) list(c *gin.Context) {
	ctx := c.Request.Context()
	var (
		incidents []domain.Incident
		err       error
	)
	if c.Query("active") == "true" {
		incidents, err = h.queries.ListActive(ctx, h.wsID(c))
	} else {
		incidents, err = h.queries.List(ctx, h.wsID(c))
	}
	if err != nil {
		failDomain(c, err)
		return
	}
	out := make([]incidentResponse, 0, len(incidents))
	for _, inc := range incidents {
		out = append(out, toResponse(inc))
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *Handler) get(c *gin.Context) {
	inc, err := h.queries.Get(c.Request.Context(), h.wsID(c), c.Param("id"))
	if err != nil {
		failDomain(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, toResponse(inc))
}

func (h *Handler) timeline(c *gin.Context) {
	entries, err := h.queries.Timeline(c.Request.Context(), h.wsID(c), c.Param("id"))
	if err != nil {
		failDomain(c, err)
		return
	}
	out := make([]timelineResponse, 0, len(entries))
	for _, e := range entries {
		out = append(out, toTimeline(e))
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *Handler) triage(c *gin.Context) {
	var req triageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	inc, err := h.transitions.Triage(c.Request.Context(), h.wsID(c), c.Param("id"), req.Severity, req.Description, actorFrom(c))
	respond(c, inc, err)
}

func (h *Handler) assign(c *gin.Context) {
	var req assignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	inc, err := h.transitions.Assign(c.Request.Context(), h.wsID(c), c.Param("id"), req.Assignee, actorFrom(c))
	respond(c, inc, err)
}

func (h *Handler) mitigate(c *gin.Context) {
	inc, err := h.transitions.StartMitigation(c.Request.Context(), h.wsID(c), c.Param("id"), actorFrom(c))
	respond(c, inc, err)
}

func (h *Handler) resolve(c *gin.Context) {
	var req noteRequest
	_ = c.ShouldBindJSON(&req) // note tùy chọn
	inc, err := h.transitions.Resolve(c.Request.Context(), h.wsID(c), c.Param("id"), req.Note, actorFrom(c))
	respond(c, inc, err)
}

func (h *Handler) postmortem(c *gin.Context) {
	inc, err := h.transitions.RequirePostmortem(c.Request.Context(), h.wsID(c), c.Param("id"), actorFrom(c))
	respond(c, inc, err)
}

func (h *Handler) close(c *gin.Context) {
	var req noteRequest
	_ = c.ShouldBindJSON(&req) // note tùy chọn
	inc, err := h.transitions.Close(c.Request.Context(), h.wsID(c), c.Param("id"), req.Note, actorFrom(c))
	respond(c, inc, err)
}

func respond(c *gin.Context, inc domain.Incident, err error) {
	if err != nil {
		failDomain(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, toResponse(inc))
}

// failDomain map domain error sang HTTP status (message generic an toàn).
func failDomain(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrIncidentNotFound):
		httpx.Fail(c, http.StatusNotFound, "incident not found")
	case errors.Is(err, domain.ErrScheduleNotFound):
		httpx.Fail(c, http.StatusNotFound, "on-call schedule not found")
	case errors.Is(err, domain.ErrEscalationPolicyNotFound):
		httpx.Fail(c, http.StatusNotFound, "escalation policy not found")
	case errors.Is(err, domain.ErrPostmortemNotFound):
		httpx.Fail(c, http.StatusNotFound, "postmortem not found")
	case errors.Is(err, domain.ErrActionItemNotFound):
		httpx.Fail(c, http.StatusNotFound, "action item not found")
	case errors.Is(err, domain.ErrPostmortemPublished):
		httpx.Fail(c, http.StatusConflict, "postmortem already published")
	case errors.Is(err, domain.ErrInvalidTransition):
		httpx.Fail(c, http.StatusConflict, err.Error())
	default:
		var ve *domain.ValidationError
		if errors.As(err, &ve) {
			httpx.Fail(c, http.StatusBadRequest, ve.Error())
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, "internal server error")
	}
}

// wsID lấy workspace từ context (đã qua RequireAuthWorkspace); fallback sang
// workspace mặc định khi không có context (webhook machine-auth / test).
func (h *Handler) wsID(c *gin.Context) string {
	if ws, ok := auth.WorkspaceIDFromContext(c); ok {
		return ws
	}
	return h.workspaceID
}
