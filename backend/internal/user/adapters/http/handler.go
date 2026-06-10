// Package http expose use case của user qua REST API dùng Gin. Validate request
// bằng validator/v10 và trả response qua envelope chuẩn (httpx).
package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"github.com/namdam97/logmon/backend/internal/shared/httpx"
	"github.com/namdam97/logmon/backend/internal/user/app"
	"github.com/namdam97/logmon/backend/internal/user/domain"
)

// Handler gắn các use case của user vào HTTP routes.
type Handler struct {
	svc      *app.Service
	validate *validator.Validate
}

// NewHandler tạo Handler với application service.
func NewHandler(svc *app.Service) *Handler {
	return &Handler{svc: svc, validate: validator.New(validator.WithRequiredStructEnabled())}
}

// Register gắn routes của user vào router group cho trước.
func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.POST("/users", h.register)
	rg.GET("/users/:id", h.get)
}

type registerRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8,max=72"`
}

type userResponse struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	CreatedAt string `json:"createdAt"`
}

func toResponse(u domain.User) userResponse {
	return userResponse{
		ID:        u.ID().String(),
		Email:     u.Email().String(),
		CreatedAt: u.CreatedAt().Format(http.TimeFormat),
	}
}

func (h *Handler) register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.validate.Struct(req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request fields")
		return
	}

	user, err := h.svc.Register(c.Request.Context(), app.RegisterInput{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		failDomain(c, err)
		return
	}
	httpx.OK(c, http.StatusCreated, toResponse(user))
}

func (h *Handler) get(c *gin.Context) {
	user, err := h.svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		failDomain(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, toResponse(user))
}

// failDomain map domain error sang HTTP status + message generic an toàn.
func failDomain(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrUserNotFound):
		httpx.Fail(c, http.StatusNotFound, "user not found")
	case errors.Is(err, domain.ErrEmailTaken):
		httpx.Fail(c, http.StatusConflict, "email already registered")
	default:
		var ve *domain.ValidationError
		if errors.As(err, &ve) {
			httpx.Fail(c, http.StatusBadRequest, ve.Error())
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, "internal server error")
	}
}
