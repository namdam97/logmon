// Package http expose use case của user qua REST API dùng Gin. Validate request
// bằng validator/v10 và trả response qua envelope chuẩn (httpx).
package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"github.com/namdam97/logmon/backend/internal/shared/auth"
	"github.com/namdam97/logmon/backend/internal/shared/httpx"
	"github.com/namdam97/logmon/backend/internal/user/app"
	"github.com/namdam97/logmon/backend/internal/user/domain"
)

// CookieConfig cấu hình cookie chứa access token.
type CookieConfig struct {
	// Secure đặt cờ Secure (chỉ gửi qua HTTPS). Bật trong production.
	Secure bool
	// MaxAgeSeconds là thời gian sống của cookie tính bằng giây.
	MaxAgeSeconds int
}

// Handler gắn các use case của user vào HTTP routes.
type Handler struct {
	svc      *app.Service
	validate *validator.Validate
	cookie   CookieConfig
}

// NewHandler tạo Handler với application service và cấu hình cookie.
func NewHandler(svc *app.Service, cookie CookieConfig) *Handler {
	return &Handler{
		svc:      svc,
		validate: validator.New(validator.WithRequiredStructEnabled()),
		cookie:   cookie,
	}
}

// Register gắn routes của user. authMW bảo vệ route yêu cầu đăng nhập; rateMW
// throttle các route nhạy cảm (đăng ký, đăng nhập) chống brute-force/spam.
func (h *Handler) Register(rg *gin.RouterGroup, authMW, rateMW gin.HandlerFunc) {
	rg.POST("/users", rateMW, h.register)
	rg.POST("/auth/login", rateMW, h.login)
	rg.GET("/users/:id", authMW, h.get)
	rg.GET("/me", authMW, h.me)
}

type registerRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8,max=72"`
}

type loginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,max=72"`
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

func (h *Handler) login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.validate.Struct(req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request fields")
		return
	}

	user, token, err := h.svc.Login(c.Request.Context(), app.LoginInput{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		failDomain(c, err)
		return
	}

	// Cookie HttpOnly + Secure + SameSite=Strict (không gửi kèm cross-site, chống CSRF).
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(auth.CookieName, token, h.cookie.MaxAgeSeconds, "/", "", h.cookie.Secure, true)
	httpx.OK(c, http.StatusOK, toResponse(user))
}

func (h *Handler) get(c *gin.Context) {
	// Authorization: chỉ cho phép user đọc chính hồ sơ của mình (chống IDOR).
	// Khi có role admin sẽ nới quyền tại đây.
	authUserID, ok := auth.UserIDFromContext(c)
	if !ok || authUserID != c.Param("id") {
		httpx.Fail(c, http.StatusForbidden, "forbidden")
		return
	}
	user, err := h.svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		failDomain(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, toResponse(user))
}

// me trả về user đang đăng nhập dựa trên userID trong token.
func (h *Handler) me(c *gin.Context) {
	userID, ok := auth.UserIDFromContext(c)
	if !ok {
		httpx.Fail(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	user, err := h.svc.Get(c.Request.Context(), userID)
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
	case errors.Is(err, domain.ErrInvalidCredentials):
		httpx.Fail(c, http.StatusUnauthorized, "invalid credentials")
	default:
		var ve *domain.ValidationError
		if errors.As(err, &ve) {
			httpx.Fail(c, http.StatusBadRequest, ve.Error())
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, "internal server error")
	}
}
