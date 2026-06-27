// Package http expose notification BC qua REST (Gin): CRUD channel + test gửi +
// history. Secret trong config KHÔNG BAO GIỜ trả ra ngoài — chỉ liệt kê key đã
// set (configKeys). Map domain error sang HTTP status an toàn (httpx).
package http

import (
	"context"
	"errors"
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"

	"github.com/namdam97/logmon/backend/internal/notification/app/command"
	"github.com/namdam97/logmon/backend/internal/notification/domain"
	"github.com/namdam97/logmon/backend/internal/shared/auth"
	"github.com/namdam97/logmon/backend/internal/shared/httpx"
)

const timeLayout = "2006-01-02T15:04:05Z07:00"

// Use-case interfaces (ISP — handler accept interface).
type channelCreator interface {
	Handle(ctx context.Context, in command.CreateChannelInput) (domain.Channel, error)
}
type channelUpdater interface {
	Handle(ctx context.Context, in command.UpdateChannelInput) (domain.Channel, error)
}
type channelDeleter interface {
	Handle(ctx context.Context, workspaceID, id string) error
}
type channelTester interface {
	Handle(ctx context.Context, workspaceID, id string) error
}
type channelReader interface {
	ByID(ctx context.Context, workspaceID string, id domain.ChannelID) (domain.Channel, error)
	List(ctx context.Context, workspaceID string) ([]domain.Channel, error)
}
type historyReader interface {
	List(ctx context.Context, workspaceID string, limit int) ([]domain.HistoryEntry, error)
}

// Handler gắn use case notification vào HTTP routes.
type Handler struct {
	creator     channelCreator
	updater     channelUpdater
	deleter     channelDeleter
	tester      channelTester
	reader      channelReader
	history     historyReader
	workspaceID string
}

// NewHandler tạo Handler. workspaceID là workspace mặc định (multi-tenancy 3.6).
func NewHandler(creator channelCreator, updater channelUpdater, deleter channelDeleter, tester channelTester, reader channelReader, history historyReader, workspaceID string) *Handler {
	return &Handler{
		creator: creator, updater: updater, deleter: deleter, tester: tester,
		reader: reader, history: history, workspaceID: workspaceID,
	}
}

// Register gắn routes. authMW bảo vệ mọi route.
func (h *Handler) Register(rg *gin.RouterGroup, authMW gin.HandlerFunc) {
	rg.POST("/notifications/channels", authMW, h.create)
	rg.GET("/notifications/channels", authMW, h.list)
	rg.GET("/notifications/history", authMW, h.listHistory)
	rg.GET("/notifications/channels/:id", authMW, h.get)
	rg.PUT("/notifications/channels/:id", authMW, h.update)
	rg.DELETE("/notifications/channels/:id", authMW, h.delete)
	rg.POST("/notifications/channels/:id/test", authMW, h.test)
}

type channelRequest struct {
	Name        string            `json:"name"`
	ChannelType string            `json:"channelType"`
	Config      map[string]string `json:"config"`
	Events      []string          `json:"events"`
	Enabled     bool              `json:"enabled"`
}

type channelResponse struct {
	ID          string   `json:"id"`
	WorkspaceID string   `json:"workspaceId"`
	Name        string   `json:"name"`
	ChannelType string   `json:"channelType"`
	ConfigKeys  []string `json:"configKeys"` // chỉ tên key đã set — KHÔNG lộ secret
	Events      []string `json:"events"`
	Enabled     bool     `json:"enabled"`
	CreatedAt   string   `json:"createdAt"`
	UpdatedAt   string   `json:"updatedAt"`
}

func toResponse(c domain.Channel) channelResponse {
	cfg := c.Config()
	keys := make([]string, 0, len(cfg))
	for k := range cfg {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return channelResponse{
		ID:          c.ID().String(),
		WorkspaceID: c.WorkspaceID(),
		Name:        c.Name(),
		ChannelType: c.Type().String(),
		ConfigKeys:  keys,
		Events:      c.Events(),
		Enabled:     c.IsEnabled(),
		CreatedAt:   c.CreatedAt().Format(timeLayout),
		UpdatedAt:   c.UpdatedAt().Format(timeLayout),
	}
}

func (h *Handler) create(c *gin.Context) {
	var req channelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	ch, err := h.creator.Handle(c.Request.Context(), command.CreateChannelInput{
		WorkspaceID: h.wsID(c),
		Name:        req.Name,
		ChannelType: req.ChannelType,
		Config:      req.Config,
		Events:      req.Events,
	})
	if err != nil {
		failDomain(c, err)
		return
	}
	httpx.OK(c, http.StatusCreated, toResponse(ch))
}

func (h *Handler) list(c *gin.Context) {
	channels, err := h.reader.List(c.Request.Context(), h.wsID(c))
	if err != nil {
		failDomain(c, err)
		return
	}
	out := make([]channelResponse, 0, len(channels))
	for _, ch := range channels {
		out = append(out, toResponse(ch))
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *Handler) get(c *gin.Context) {
	id, err := domain.NewChannelID(c.Param("id"))
	if err != nil {
		failDomain(c, err)
		return
	}
	ch, err := h.reader.ByID(c.Request.Context(), h.wsID(c), id)
	if err != nil {
		failDomain(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, toResponse(ch))
}

func (h *Handler) update(c *gin.Context) {
	var req channelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	ch, err := h.updater.Handle(c.Request.Context(), command.UpdateChannelInput{
		WorkspaceID: h.wsID(c),
		ID:          c.Param("id"),
		Name:        req.Name,
		ChannelType: req.ChannelType,
		Config:      req.Config,
		Events:      req.Events,
		Enabled:     req.Enabled,
	})
	if err != nil {
		failDomain(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, toResponse(ch))
}

func (h *Handler) delete(c *gin.Context) {
	if err := h.deleter.Handle(c.Request.Context(), h.wsID(c), c.Param("id")); err != nil {
		failDomain(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) test(c *gin.Context) {
	err := h.tester.Handle(c.Request.Context(), h.wsID(c), c.Param("id"))
	if err != nil {
		if errors.Is(err, domain.ErrChannelNotFound) {
			httpx.Fail(c, http.StatusNotFound, "channel not found")
			return
		}
		var ve *domain.ValidationError
		if errors.As(err, &ve) {
			httpx.Fail(c, http.StatusBadRequest, ve.Error())
			return
		}
		// Lỗi gửi đi (kênh ngoài) → 502, không phải lỗi nội bộ.
		httpx.Fail(c, http.StatusBadGateway, "test notification failed")
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{"status": "sent"})
}

type historyResponse struct {
	ChannelID    string `json:"channelId"`
	EventType    string `json:"eventType"`
	EventRef     string `json:"eventRef"`
	Status       string `json:"status"`
	ResponseCode int    `json:"responseCode,omitempty"`
	ErrorMessage string `json:"errorMessage,omitempty"`
	SentAt       string `json:"sentAt"`
}

func (h *Handler) listHistory(c *gin.Context) {
	entries, err := h.history.List(c.Request.Context(), h.wsID(c), 0)
	if err != nil {
		failDomain(c, err)
		return
	}
	out := make([]historyResponse, 0, len(entries))
	for _, e := range entries {
		out = append(out, historyResponse{
			ChannelID:    e.ChannelID,
			EventType:    e.EventType,
			EventRef:     e.EventRef,
			Status:       string(e.Status),
			ResponseCode: e.ResponseCode,
			ErrorMessage: e.ErrorMessage,
			SentAt:       e.SentAt.Format(timeLayout),
		})
	}
	httpx.OK(c, http.StatusOK, out)
}

// failDomain map domain error sang HTTP status (message generic an toàn).
func failDomain(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrChannelNotFound):
		httpx.Fail(c, http.StatusNotFound, "channel not found")
	case errors.Is(err, domain.ErrChannelNameTaken):
		httpx.Fail(c, http.StatusConflict, "channel name already taken")
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
