package http

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/namdam97/logmon/backend/internal/incident/app/command"
	"github.com/namdam97/logmon/backend/internal/incident/domain"
	"github.com/namdam97/logmon/backend/internal/shared/auth"
	"github.com/namdam97/logmon/backend/internal/shared/httpx"
)

// On-call & escalation HTTP (doc_v2/06 §1.4): tạo/list schedule, "ai đang on-call",
// override (swap/nghỉ phép), cấu hình escalation policy.

// Use-case interfaces (ISP).
type scheduleCreator interface {
	Handle(ctx context.Context, in command.CreateScheduleInput) (domain.Schedule, error)
}
type overrideCreator interface {
	Handle(ctx context.Context, in command.CreateOverrideInput) (domain.Override, error)
}
type policyCreator interface {
	Handle(ctx context.Context, in command.CreateEscalationPolicyInput) (domain.EscalationPolicy, error)
}
type onCallQueries interface {
	ListSchedules(ctx context.Context, workspaceID string) ([]domain.Schedule, error)
	Current(ctx context.Context, workspaceID, scheduleID string, at time.Time) (domain.Schedule, domain.OnCall, error)
}

// OnCallHandler gắn use case on-call/escalation vào HTTP routes.
type OnCallHandler struct {
	schedules   scheduleCreator
	overrides   overrideCreator
	policies    policyCreator
	queries     onCallQueries
	workspaceID string
}

// NewOnCallHandler tạo handler. workspaceID là workspace mặc định (multi-tenancy 3.6).
func NewOnCallHandler(schedules scheduleCreator, overrides overrideCreator, policies policyCreator, queries onCallQueries, workspaceID string) *OnCallHandler {
	return &OnCallHandler{schedules: schedules, overrides: overrides, policies: policies, queries: queries, workspaceID: workspaceID}
}

// Register gắn routes. authMW bảo vệ mọi route.
func (h *OnCallHandler) Register(rg *gin.RouterGroup, authMW gin.HandlerFunc) {
	rg.POST("/oncall/schedules", authMW, h.createSchedule)
	rg.GET("/oncall/schedules", authMW, h.listSchedules)
	rg.GET("/oncall/schedules/:id/current", authMW, h.current)
	rg.POST("/oncall/override", authMW, h.createOverride)
	rg.POST("/oncall/escalation-policy", authMW, h.createPolicy)
}

// ---- requests / responses ----

type createScheduleRequest struct {
	Name         string   `json:"name"`
	Rotation     string   `json:"rotation"`
	Participants []string `json:"participants"`
	Timezone     string   `json:"timezone"`
	HandoffHour  int      `json:"handoffHour"`
	HandoffMin   int      `json:"handoffMin"`
	StartDate    string   `json:"startDate"` // RFC3339 hoặc YYYY-MM-DD
}

type createOverrideRequest struct {
	ScheduleID  string `json:"scheduleId"`
	Participant string `json:"participant"`
	StartAt     string `json:"startAt"` // RFC3339
	EndAt       string `json:"endAt"`   // RFC3339
}

type policyLevelRequest struct {
	Target         string `json:"target"`
	TimeoutMinutes int    `json:"timeoutMinutes"`
}

type createPolicyRequest struct {
	Name     string               `json:"name"`
	TeamLead string               `json:"teamLead"`
	Levels   []policyLevelRequest `json:"levels"`
}

type scheduleResponse struct {
	ID           string   `json:"id"`
	WorkspaceID  string   `json:"workspaceId"`
	Name         string   `json:"name"`
	Rotation     string   `json:"rotation"`
	Participants []string `json:"participants"`
	Timezone     string   `json:"timezone,omitempty"`
	Anchor       string   `json:"anchor"`
}

func toSchedule(s domain.Schedule) scheduleResponse {
	return scheduleResponse{
		ID:           s.ID().String(),
		WorkspaceID:  s.WorkspaceID(),
		Name:         s.Name(),
		Rotation:     s.Rotation().String(),
		Participants: s.Participants(),
		Timezone:     s.Timezone(),
		Anchor:       s.Anchor().UTC().Format(timeLayout),
	}
}

type onCallResponse struct {
	ScheduleID string `json:"scheduleId"`
	Primary    string `json:"primary"`
	Secondary  string `json:"secondary,omitempty"`
	OverrideID string `json:"overrideId,omitempty"`
	At         string `json:"at"`
}

type overrideResponse struct {
	ID          string `json:"id"`
	ScheduleID  string `json:"scheduleId"`
	Participant string `json:"participant"`
	StartAt     string `json:"startAt"`
	EndAt       string `json:"endAt"`
}

type policyLevelResponse struct {
	Target         string `json:"target"`
	TimeoutMinutes int    `json:"timeoutMinutes"`
}

type policyResponse struct {
	ID          string                `json:"id"`
	WorkspaceID string                `json:"workspaceId"`
	Name        string                `json:"name"`
	TeamLead    string                `json:"teamLead,omitempty"`
	Levels      []policyLevelResponse `json:"levels"`
}

func toPolicy(p domain.EscalationPolicy) policyResponse {
	levels := make([]policyLevelResponse, 0, len(p.Levels()))
	for _, lv := range p.Levels() {
		levels = append(levels, policyLevelResponse{
			Target:         lv.Target().String(),
			TimeoutMinutes: int(lv.Timeout() / time.Minute),
		})
	}
	return policyResponse{
		ID:          p.ID(),
		WorkspaceID: p.WorkspaceID(),
		Name:        p.Name(),
		TeamLead:    p.TeamLead(),
		Levels:      levels,
	}
}

// ---- handlers ----

func (h *OnCallHandler) createSchedule(c *gin.Context) {
	var req createScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	startDate, err := parseDate(req.StartDate)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid startDate (want RFC3339 or YYYY-MM-DD)")
		return
	}
	s, err := h.schedules.Handle(c.Request.Context(), command.CreateScheduleInput{
		WorkspaceID:  h.wsID(c),
		Name:         req.Name,
		Rotation:     req.Rotation,
		Participants: req.Participants,
		Timezone:     req.Timezone,
		HandoffHour:  req.HandoffHour,
		HandoffMin:   req.HandoffMin,
		StartDate:    startDate,
	})
	if err != nil {
		failDomain(c, err)
		return
	}
	httpx.OK(c, http.StatusCreated, toSchedule(s))
}

func (h *OnCallHandler) listSchedules(c *gin.Context) {
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

func (h *OnCallHandler) current(c *gin.Context) {
	at := time.Now().UTC()
	if raw := c.Query("at"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			httpx.Fail(c, http.StatusBadRequest, "invalid at (want RFC3339)")
			return
		}
		at = parsed.UTC()
	}
	_, oncall, err := h.queries.Current(c.Request.Context(), h.wsID(c), c.Param("id"), at)
	if err != nil {
		failDomain(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, onCallResponse{
		ScheduleID: c.Param("id"),
		Primary:    oncall.Primary,
		Secondary:  oncall.Secondary,
		OverrideID: oncall.OverrideID,
		At:         at.Format(timeLayout),
	})
}

func (h *OnCallHandler) createOverride(c *gin.Context) {
	var req createOverrideRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	startAt, err1 := time.Parse(time.RFC3339, req.StartAt)
	endAt, err2 := time.Parse(time.RFC3339, req.EndAt)
	if err1 != nil || err2 != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid startAt/endAt (want RFC3339)")
		return
	}
	o, err := h.overrides.Handle(c.Request.Context(), command.CreateOverrideInput{
		ScheduleID:  req.ScheduleID,
		Participant: req.Participant,
		StartAt:     startAt.UTC(),
		EndAt:       endAt.UTC(),
	})
	if err != nil {
		failDomain(c, err)
		return
	}
	httpx.OK(c, http.StatusCreated, overrideResponse{
		ID:          o.ID(),
		ScheduleID:  o.ScheduleID().String(),
		Participant: o.Participant(),
		StartAt:     o.StartAt().Format(timeLayout),
		EndAt:       o.EndAt().Format(timeLayout),
	})
}

func (h *OnCallHandler) createPolicy(c *gin.Context) {
	var req createPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	levels := make([]command.EscalationLevelInput, 0, len(req.Levels))
	for _, lv := range req.Levels {
		levels = append(levels, command.EscalationLevelInput{Target: lv.Target, TimeoutMinutes: lv.TimeoutMinutes})
	}
	p, err := h.policies.Handle(c.Request.Context(), command.CreateEscalationPolicyInput{
		WorkspaceID: h.wsID(c),
		Name:        req.Name,
		TeamLead:    req.TeamLead,
		Levels:      levels,
	})
	if err != nil {
		failDomain(c, err)
		return
	}
	httpx.OK(c, http.StatusCreated, toPolicy(p))
}

// parseDate parse RFC3339 hoặc YYYY-MM-DD.
func parseDate(raw string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", raw)
}

// wsID lấy workspace từ context (đã qua RequireAuthWorkspace); fallback sang
// workspace mặc định khi không có context (webhook machine-auth / test).
func (h *OnCallHandler) wsID(c *gin.Context) string {
	if ws, ok := auth.WorkspaceIDFromContext(c); ok {
		return ws
	}
	return h.workspaceID
}
