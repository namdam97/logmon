// Package http expose alerting rule CRUD qua REST (Gin). Validate request bằng
// validator/v10, trả response qua envelope chuẩn (httpx) và map domain error
// sang HTTP status an toàn (không leak chi tiết nội bộ).
package http

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"github.com/namdam97/logmon/backend/internal/alerting/app/command"
	"github.com/namdam97/logmon/backend/internal/alerting/domain"
	"github.com/namdam97/logmon/backend/internal/shared/httpx"
)

// ruleCreator là write-side use case mà handler phụ thuộc (ISP — accept interface).
type ruleCreator interface {
	Handle(ctx context.Context, in command.CreateRuleInput) (domain.AlertRule, error)
}

// ruleReader là read-side use case (CQRS) mà handler phụ thuộc.
type ruleReader interface {
	Get(ctx context.Context, id string) (domain.AlertRule, error)
	List(ctx context.Context, workspaceID string) ([]domain.AlertRule, error)
}

// Handler gắn use case alerting vào HTTP routes.
type Handler struct {
	creator     ruleCreator
	queries     ruleReader
	validate    *validator.Validate
	workspaceID string
}

// NewHandler tạo Handler. workspaceID là workspace mặc định GĐ2 (multi-tenancy ở GĐ3).
func NewHandler(creator ruleCreator, queries ruleReader, workspaceID string) *Handler {
	return &Handler{
		creator:     creator,
		queries:     queries,
		validate:    validator.New(validator.WithRequiredStructEnabled()),
		workspaceID: workspaceID,
	}
}

// Register gắn routes alerting. authMW bảo vệ mọi route (yêu cầu đăng nhập).
func (h *Handler) Register(rg *gin.RouterGroup, authMW gin.HandlerFunc) {
	rg.POST("/alert-rules", authMW, h.create)
	rg.GET("/alert-rules", authMW, h.list)
	rg.GET("/alert-rules/:id", authMW, h.get)
}

type createRuleRequest struct {
	Name        string            `json:"name" validate:"required,max=100"`
	Expression  string            `json:"expression" validate:"required"`
	Service     string            `json:"service" validate:"required,max=100"`
	Severity    string            `json:"severity" validate:"required,oneof=critical warning info"`
	ForDuration string            `json:"forDuration" validate:"required"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations" validate:"required"`
}

type ruleResponse struct {
	ID          string            `json:"id"`
	WorkspaceID string            `json:"workspaceId"`
	Name        string            `json:"name"`
	Expression  string            `json:"expression"`
	Service     string            `json:"service"`
	Severity    string            `json:"severity"`
	ForDuration string            `json:"forDuration"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	Enabled     bool              `json:"enabled"`
	SyncStatus  string            `json:"syncStatus"`
	CreatedAt   string            `json:"createdAt"`
	UpdatedAt   string            `json:"updatedAt"`
}

func toResponse(r domain.AlertRule) ruleResponse {
	return ruleResponse{
		ID:          r.ID().String(),
		WorkspaceID: r.WorkspaceID(),
		Name:        r.Name(),
		Expression:  r.Expression(),
		Service:     r.Service(),
		Severity:    r.Severity().String(),
		ForDuration: r.ForDuration().String(),
		Labels:      r.Labels(),
		Annotations: r.Annotations(),
		Enabled:     r.IsEnabled(),
		SyncStatus:  string(r.SyncStatus()),
		CreatedAt:   r.CreatedAt().Format(time.RFC3339),
		UpdatedAt:   r.UpdatedAt().Format(time.RFC3339),
	}
}

func (h *Handler) create(c *gin.Context) {
	var req createRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.validate.Struct(req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request fields")
		return
	}
	dur, err := time.ParseDuration(req.ForDuration)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid forDuration: must be a Go duration like 5m")
		return
	}

	rule, err := h.creator.Handle(c.Request.Context(), command.CreateRuleInput{
		WorkspaceID: h.workspaceID,
		Name:        req.Name,
		Expression:  req.Expression,
		Service:     req.Service,
		Severity:    req.Severity,
		ForDuration: dur,
		Labels:      req.Labels,
		Annotations: req.Annotations,
	})
	if err != nil {
		failDomain(c, err)
		return
	}
	httpx.OK(c, http.StatusCreated, toResponse(rule))
}

func (h *Handler) get(c *gin.Context) {
	rule, err := h.queries.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		failDomain(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, toResponse(rule))
}

func (h *Handler) list(c *gin.Context) {
	rules, err := h.queries.List(c.Request.Context(), h.workspaceID)
	if err != nil {
		failDomain(c, err)
		return
	}
	out := make([]ruleResponse, 0, len(rules))
	for _, r := range rules {
		out = append(out, toResponse(r))
	}
	httpx.OK(c, http.StatusOK, out)
}

// failDomain map domain error sang HTTP status + message generic an toàn.
func failDomain(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrRuleNotFound):
		httpx.Fail(c, http.StatusNotFound, "alert rule not found")
	case errors.Is(err, domain.ErrRuleNameTaken):
		httpx.Fail(c, http.StatusConflict, "alert rule name already taken")
	default:
		var ve *domain.ValidationError
		if errors.As(err, &ve) {
			httpx.Fail(c, http.StatusBadRequest, ve.Error())
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, "internal server error")
	}
}
