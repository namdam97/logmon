package http

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/namdam97/logmon/backend/internal/shared/audit"
	"github.com/namdam97/logmon/backend/internal/shared/auth"
	"github.com/namdam97/logmon/backend/internal/shared/httpx"
	"github.com/namdam97/logmon/backend/internal/user/app"
	"github.com/namdam97/logmon/backend/internal/user/domain"
)

// Use-case interfaces (ISP).
type workspaceService interface {
	Create(ctx context.Context, in app.CreateWorkspaceInput) (domain.Workspace, error)
	ListForUser(ctx context.Context, userID string) ([]domain.Workspace, error)
}
type memberService interface {
	List(ctx context.Context, workspaceID string) ([]domain.Membership, error)
	Add(ctx context.Context, in app.AddMemberInput) (domain.Membership, error)
	UpdateRole(ctx context.Context, workspaceID, userID, role string) (domain.Membership, error)
	Remove(ctx context.Context, workspaceID, userID string) error
}

// WorkspaceHandler expose workspace + RBAC member management qua REST (doc_v2/07
// §2.9). Member changes được ghi audit (immutable).
type WorkspaceHandler struct {
	workspaces workspaceService
	members    memberService
	resolver   auth.MembershipResolver
	auditor    audit.Recorder
}

// NewWorkspaceHandler tạo handler.
func NewWorkspaceHandler(workspaces workspaceService, members memberService, resolver auth.MembershipResolver, auditor audit.Recorder) *WorkspaceHandler {
	return &WorkspaceHandler{workspaces: workspaces, members: members, resolver: resolver, auditor: auditor}
}

// Register gắn routes. authMW yêu cầu đăng nhập; member routes thêm RBAC theo
// workspace ở path (RequireWorkspaceParam + RequireRole).
func (h *WorkspaceHandler) Register(rg *gin.RouterGroup, authMW gin.HandlerFunc) {
	rg.GET("/workspaces", authMW, h.list)
	rg.POST("/workspaces", authMW, h.create)

	wsParam := auth.RequireWorkspaceParam(h.resolver, "id")
	rg.GET("/workspaces/:id/members", authMW, wsParam, auth.RequireRole(auth.RoleViewer), h.listMembers)
	rg.POST("/workspaces/:id/members", authMW, wsParam, auth.RequireRole(auth.RoleAdmin), h.addMember)
	rg.PUT("/workspaces/:id/members/:uid", authMW, wsParam, auth.RequireRole(auth.RoleAdmin), h.updateMember)
	rg.DELETE("/workspaces/:id/members/:uid", authMW, wsParam, auth.RequireRole(auth.RoleAdmin), h.removeMember)
}

// ---- requests / responses ----

type createWorkspaceRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type addMemberRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type updateMemberRequest struct {
	Role string `json:"role"`
}

type workspaceResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type memberResponse struct {
	WorkspaceID string `json:"workspaceId"`
	UserID      string `json:"userId"`
	Role        string `json:"role"`
	JoinedAt    string `json:"joinedAt"`
}

func toWorkspace(w domain.Workspace) workspaceResponse {
	return workspaceResponse{
		ID:        w.ID().String(),
		Name:      w.Name(),
		Slug:      w.Slug(),
		CreatedAt: w.CreatedAt().Format(http.TimeFormat),
		UpdatedAt: w.UpdatedAt().Format(http.TimeFormat),
	}
}

func toMember(m domain.Membership) memberResponse {
	return memberResponse{
		WorkspaceID: m.WorkspaceID().String(),
		UserID:      m.UserID().String(),
		Role:        m.Role().String(),
		JoinedAt:    m.JoinedAt().Format(http.TimeFormat),
	}
}

// ---- handlers ----

func (h *WorkspaceHandler) list(c *gin.Context) {
	userID, ok := auth.UserIDFromContext(c)
	if !ok {
		httpx.Fail(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	wss, err := h.workspaces.ListForUser(c.Request.Context(), userID)
	if err != nil {
		failDomain(c, err)
		return
	}
	out := make([]workspaceResponse, 0, len(wss))
	for _, w := range wss {
		out = append(out, toWorkspace(w))
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *WorkspaceHandler) create(c *gin.Context) {
	userID, ok := auth.UserIDFromContext(c)
	if !ok {
		httpx.Fail(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req createWorkspaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	ws, err := h.workspaces.Create(c.Request.Context(), app.CreateWorkspaceInput{
		Name: req.Name, Slug: req.Slug, OwnerUserID: userID,
	})
	if err != nil {
		failDomain(c, err)
		return
	}
	h.audit(c, "workspace.create", "workspace", ws.ID().String(), map[string]any{"slug": ws.Slug()})
	httpx.OK(c, http.StatusCreated, toWorkspace(ws))
}

func (h *WorkspaceHandler) listMembers(c *gin.Context) {
	members, err := h.members.List(c.Request.Context(), c.Param("id"))
	if err != nil {
		failDomain(c, err)
		return
	}
	out := make([]memberResponse, 0, len(members))
	for _, m := range members {
		out = append(out, toMember(m))
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *WorkspaceHandler) addMember(c *gin.Context) {
	var req addMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	m, err := h.members.Add(c.Request.Context(), app.AddMemberInput{
		WorkspaceID: c.Param("id"), Email: req.Email, Role: req.Role,
	})
	if err != nil {
		failDomain(c, err)
		return
	}
	h.audit(c, "member.add", "membership", m.UserID().String(), map[string]any{"role": m.Role().String()})
	httpx.OK(c, http.StatusCreated, toMember(m))
}

func (h *WorkspaceHandler) updateMember(c *gin.Context) {
	var req updateMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	m, err := h.members.UpdateRole(c.Request.Context(), c.Param("id"), c.Param("uid"), req.Role)
	if err != nil {
		failDomain(c, err)
		return
	}
	h.audit(c, "member.update", "membership", m.UserID().String(), map[string]any{"role": m.Role().String()})
	httpx.OK(c, http.StatusOK, toMember(m))
}

func (h *WorkspaceHandler) removeMember(c *gin.Context) {
	if err := h.members.Remove(c.Request.Context(), c.Param("id"), c.Param("uid")); err != nil {
		failDomain(c, err)
		return
	}
	h.audit(c, "member.remove", "membership", c.Param("uid"), nil)
	httpx.OK(c, http.StatusOK, gin.H{"removed": true})
}

// audit ghi best-effort: lỗi audit không chặn nghiệp vụ (log nội bộ qua envelope).
func (h *WorkspaceHandler) audit(c *gin.Context, action, resourceType, resourceID string, details map[string]any) {
	if h.auditor == nil {
		return
	}
	userID, _ := auth.UserIDFromContext(c)
	wsID, _ := auth.WorkspaceIDFromContext(c)
	_ = h.auditor.Record(c.Request.Context(), audit.Entry{
		WorkspaceID:  wsID,
		UserID:       userID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Details:      details,
		IPAddress:    c.ClientIP(),
	})
}
