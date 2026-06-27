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
	"github.com/namdam97/logmon/backend/internal/shared/auth"
	"github.com/namdam97/logmon/backend/internal/shared/httpx"
)

// silenceCreator là write-side use case tạo silence (ISP).
type silenceCreator interface {
	Handle(ctx context.Context, in command.CreateSilenceInput) (string, error)
}

// silenceDeleter là write-side use case huỷ silence (ISP).
type silenceDeleter interface {
	Handle(ctx context.Context, id string) error
}

// silenceLister là read-side use case liệt kê silence (CQRS).
type silenceLister interface {
	List(ctx context.Context) ([]domain.SilenceView, error)
}

// SilenceHandler proxy silence sang Alertmanager qua REST. Mọi route yêu cầu
// người dùng đã đăng nhập (createdBy lấy từ JWT, không nhận từ body — chống giả mạo).
type SilenceHandler struct {
	creator  silenceCreator
	deleter  silenceDeleter
	lister   silenceLister
	validate *validator.Validate
}

// NewSilenceHandler tạo handler với use case được inject.
func NewSilenceHandler(creator silenceCreator, deleter silenceDeleter, lister silenceLister) *SilenceHandler {
	return &SilenceHandler{
		creator:  creator,
		deleter:  deleter,
		lister:   lister,
		validate: validator.New(validator.WithRequiredStructEnabled()),
	}
}

// Register gắn routes. authMW bảo vệ mọi route (người dùng đã đăng nhập).
func (h *SilenceHandler) Register(rg *gin.RouterGroup, authMW gin.HandlerFunc) {
	editor := auth.RequireRole(auth.RoleEditor)
	rg.POST("/alerts/silences", authMW, editor, h.create)
	rg.GET("/alerts/silences", authMW, h.list)
	rg.DELETE("/alerts/silences/:id", authMW, editor, h.delete)
}

type matcherRequest struct {
	Name    string `json:"name" validate:"required"`
	Value   string `json:"value"`
	IsRegex bool   `json:"isRegex"`
	// IsEqual mặc định true (khớp dương) khi client bỏ trống — dùng con trỏ để
	// phân biệt "không gửi" với "gửi false" (phủ định).
	IsEqual *bool `json:"isEqual"`
}

type createSilenceRequest struct {
	Matchers []matcherRequest `json:"matchers" validate:"required,min=1,dive"`
	StartsAt *time.Time       `json:"startsAt"`
	EndsAt   time.Time        `json:"endsAt" validate:"required"`
	Comment  string           `json:"comment" validate:"required,max=500"`
}

func (h *SilenceHandler) create(c *gin.Context) {
	userID, ok := auth.UserIDFromContext(c)
	if !ok {
		httpx.Fail(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req createSilenceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.validate.Struct(req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request fields")
		return
	}

	matchers := make([]domain.MatcherInput, 0, len(req.Matchers))
	for _, m := range req.Matchers {
		isEqual := true
		if m.IsEqual != nil {
			isEqual = *m.IsEqual
		}
		matchers = append(matchers, domain.MatcherInput{
			Name: m.Name, Value: m.Value, IsRegex: m.IsRegex, IsEqual: isEqual,
		})
	}
	in := command.CreateSilenceInput{
		Matchers:  matchers,
		EndsAt:    req.EndsAt,
		CreatedBy: userID,
		Comment:   req.Comment,
	}
	if req.StartsAt != nil {
		in.StartsAt = *req.StartsAt
	}

	id, err := h.creator.Handle(c.Request.Context(), in)
	if err != nil {
		failSilence(c, err)
		return
	}
	httpx.OK(c, http.StatusCreated, gin.H{"silenceId": id})
}

func (h *SilenceHandler) list(c *gin.Context) {
	views, err := h.lister.List(c.Request.Context())
	if err != nil {
		failSilence(c, err)
		return
	}
	out := make([]silenceResponse, 0, len(views))
	for _, v := range views {
		out = append(out, toSilenceResponse(v))
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *SilenceHandler) delete(c *gin.Context) {
	if err := h.deleter.Handle(c.Request.Context(), c.Param("id")); err != nil {
		failSilence(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// failSilence map error sang HTTP status. Lỗi upstream (không tới được
// Alertmanager) → 502 Bad Gateway; message generic, không leak chi tiết nội bộ.
func failSilence(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrSilenceNotFound):
		httpx.Fail(c, http.StatusNotFound, "silence not found")
	default:
		var ve *domain.ValidationError
		if errors.As(err, &ve) {
			httpx.Fail(c, http.StatusBadRequest, ve.Error())
			return
		}
		httpx.Fail(c, http.StatusBadGateway, "alertmanager unavailable")
	}
}

type silenceMatcherResponse struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	IsRegex bool   `json:"isRegex"`
	IsEqual bool   `json:"isEqual"`
}

type silenceResponse struct {
	ID        string                   `json:"id"`
	Status    string                   `json:"status"`
	Matchers  []silenceMatcherResponse `json:"matchers"`
	StartsAt  string                   `json:"startsAt"`
	EndsAt    string                   `json:"endsAt"`
	CreatedBy string                   `json:"createdBy"`
	Comment   string                   `json:"comment"`
}

func toSilenceResponse(v domain.SilenceView) silenceResponse {
	matchers := make([]silenceMatcherResponse, 0, len(v.Matchers))
	for _, m := range v.Matchers {
		matchers = append(matchers, silenceMatcherResponse{
			Name: m.Name(), Value: m.Value(), IsRegex: m.IsRegex(), IsEqual: m.IsEqual(),
		})
	}
	return silenceResponse{
		ID:        v.ID,
		Status:    v.Status,
		Matchers:  matchers,
		StartsAt:  formatTime(v.StartsAt),
		EndsAt:    formatTime(v.EndsAt),
		CreatedBy: v.CreatedBy,
		Comment:   v.Comment,
	}
}
