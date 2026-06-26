// Package http expose use case của user qua REST API dùng Gin. Validate request
// bằng validator/v10 và trả response qua envelope chuẩn (httpx).
package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"github.com/namdam97/logmon/backend/internal/shared/auth"
	"github.com/namdam97/logmon/backend/internal/shared/httpx"
	"github.com/namdam97/logmon/backend/internal/user/app"
	"github.com/namdam97/logmon/backend/internal/user/domain"
)

// CookieConfig cấu hình cookie chứa access token + refresh token.
type CookieConfig struct {
	// Secure đặt cờ Secure (chỉ gửi qua HTTPS). Bật trong production.
	Secure bool
	// MaxAgeSeconds là thời gian sống của access cookie tính bằng giây.
	MaxAgeSeconds int
	// RefreshMaxAgeSeconds là thời gian sống của refresh cookie (dài hơn access).
	RefreshMaxAgeSeconds int
}

// _refreshCookieName giữ refresh token; _refreshCookiePath giới hạn cookie chỉ
// gửi tới các route auth (rotate/logout) thay vì mọi request như access cookie.
const (
	_refreshCookieName = "lm_refresh"
	_refreshCookiePath = "/api/v1/auth"
)

// refresher là các thao tác refresh-token mà handler cần (ISP).
type refresher interface {
	Issue(ctx context.Context, userID string) (string, error)
	Rotate(ctx context.Context, rawRefresh string) (app.TokenPair, error)
	Revoke(ctx context.Context, rawRefresh string) error
}

// Handler gắn các use case của user vào HTTP routes.
type Handler struct {
	svc      *app.Service
	refresh  refresher
	validate *validator.Validate
	cookie   CookieConfig
}

// NewHandler tạo Handler với application service, refresh service và cấu hình cookie.
func NewHandler(svc *app.Service, refresh refresher, cookie CookieConfig) *Handler {
	return &Handler{
		svc:      svc,
		refresh:  refresh,
		validate: validator.New(validator.WithRequiredStructEnabled()),
		cookie:   cookie,
	}
}

// Register gắn routes của user. authMW bảo vệ route yêu cầu đăng nhập; rateMW
// throttle các route nhạy cảm (đăng ký, đăng nhập, refresh) chống brute-force/spam.
func (h *Handler) Register(rg *gin.RouterGroup, authMW, rateMW gin.HandlerFunc) {
	rg.POST("/users", rateMW, h.register)
	rg.POST("/auth/login", rateMW, h.login)
	rg.POST("/auth/refresh", rateMW, h.refreshToken)
	rg.POST("/auth/logout", h.logout)
	rg.GET("/users/:id", authMW, h.get)
	rg.GET("/me", authMW, h.me)
}

// setAuthCookies set access + refresh cookie (HttpOnly, Secure, SameSite=Strict).
func (h *Handler) setAuthCookies(c *gin.Context, access, refresh string) {
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(auth.CookieName, access, h.cookie.MaxAgeSeconds, "/", "", h.cookie.Secure, true)
	c.SetCookie(_refreshCookieName, refresh, h.cookie.RefreshMaxAgeSeconds, _refreshCookiePath, "", h.cookie.Secure, true)
}

// clearAuthCookies xoá cả hai cookie (maxAge < 0).
func (h *Handler) clearAuthCookies(c *gin.Context) {
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(auth.CookieName, "", -1, "/", "", h.cookie.Secure, true)
	c.SetCookie(_refreshCookieName, "", -1, _refreshCookiePath, "", h.cookie.Secure, true)
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

	refresh, err := h.refresh.Issue(c.Request.Context(), user.ID().String())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "internal server error")
		return
	}
	h.setAuthCookies(c, token, refresh)
	httpx.OK(c, http.StatusOK, toResponse(user))
}

// refreshToken rotate refresh token lấy cặp access+refresh mới. Token cũ bị vô
// hiệu; phát hiện reuse → thu hồi family + xoá cookie. Đọc refresh từ cookie
// (HttpOnly) — JS không truy cập được.
func (h *Handler) refreshToken(c *gin.Context) {
	raw, err := c.Cookie(_refreshCookieName)
	if err != nil || raw == "" {
		httpx.Fail(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	pair, err := h.refresh.Rotate(c.Request.Context(), raw)
	if err != nil {
		// Reuse hoặc token không hợp lệ: xoá cookie, trả 401 generic.
		h.clearAuthCookies(c)
		if errors.Is(err, domain.ErrRefreshTokenReused) || errors.Is(err, domain.ErrRefreshTokenInvalid) {
			httpx.Fail(c, http.StatusUnauthorized, "unauthorized")
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, "internal server error")
		return
	}
	h.setAuthCookies(c, pair.Access, pair.Refresh)
	httpx.OK(c, http.StatusOK, gin.H{"refreshed": true})
}

// logout thu hồi refresh family (nếu có) và xoá cookie phiên. Idempotent — không
// yêu cầu đăng nhập, gọi nhiều lần vô hại.
func (h *Handler) logout(c *gin.Context) {
	if raw, err := c.Cookie(_refreshCookieName); err == nil && raw != "" {
		// Best-effort: lỗi thu hồi không chặn logout (vẫn xoá cookie phía client).
		_ = h.refresh.Revoke(c.Request.Context(), raw)
	}
	h.clearAuthCookies(c)
	httpx.OK(c, http.StatusOK, gin.H{"loggedOut": true})
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
