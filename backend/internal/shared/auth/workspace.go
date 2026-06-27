package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// WorkspaceHeader là header client gửi để chọn workspace đang thao tác (GĐ3.6).
const WorkspaceHeader = "X-Workspace-ID"

const (
	contextWorkspaceIDKey = "auth_workspace_id"
	contextRoleKey        = "auth_role"
)

// Role string constants — khớp domain identity. shared kernel giữ bản sao để
// enforce RBAC cho mọi BC mà KHÔNG phụ thuộc package domain của BC nào (giữ
// hướng phụ thuộc: BC → shared, không ngược lại).
const (
	RoleViewer        = "viewer"
	RoleEditor        = "editor"
	RoleAdmin         = "admin"
	RolePlatformAdmin = "platform_admin"
)

// _roleRank xếp hạng role theo phân cấp (viewer<editor<admin<platform_admin).
var _roleRank = map[string]int{
	RoleViewer:        1,
	RoleEditor:        2,
	RoleAdmin:         3,
	RolePlatformAdmin: 4,
}

// RoleAtLeast báo role have có quyền tối thiểu bằng min không (role lạ → false).
func RoleAtLeast(have, min string) bool {
	h, ok := _roleRank[have]
	if !ok {
		return false
	}
	return h >= _roleRank[min]
}

// MembershipResolver giải quyết role của user trong workspace. ok=false nghĩa là
// không phải thành viên (middleware → 404); err chỉ dành cho lỗi hạ tầng (→500).
type MembershipResolver interface {
	Resolve(ctx context.Context, userID, workspaceID string) (role string, ok bool, err error)
}

// RequireWorkspace validate header X-Workspace-ID + membership của user (đã qua
// RequireAuth). Gắn workspaceID + role vào context cho handler/RBAC dùng. Resource
// khác workspace hoặc không phải thành viên → 404 (không lộ tồn tại — doc_v2/09).
func RequireWorkspace(resolver MembershipResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := UserIDFromContext(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "error": "unauthorized"})
			return
		}
		wsID := strings.TrimSpace(c.GetHeader(WorkspaceHeader))
		if wsID == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"success": false, "error": "workspace required"})
			return
		}
		role, ok, err := resolver.Resolve(c.Request.Context(), userID, wsID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"success": false, "error": "internal server error"})
			return
		}
		if !ok {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"success": false, "error": "not found"})
			return
		}
		c.Set(contextWorkspaceIDKey, wsID)
		c.Set(contextRoleKey, role)
		c.Next()
	}
}

// RequireWorkspaceParam giống RequireWorkspace nhưng lấy workspace từ path param
// (vd /workspaces/:id/members) thay vì header — dùng cho route thao tác trực tiếp
// trên một workspace cụ thể. Validate membership + gắn role vào context.
func RequireWorkspaceParam(resolver MembershipResolver, param string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := UserIDFromContext(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "error": "unauthorized"})
			return
		}
		wsID := strings.TrimSpace(c.Param(param))
		if wsID == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"success": false, "error": "workspace required"})
			return
		}
		role, ok, err := resolver.Resolve(c.Request.Context(), userID, wsID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"success": false, "error": "internal server error"})
			return
		}
		if !ok {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"success": false, "error": "not found"})
			return
		}
		c.Set(contextWorkspaceIDKey, wsID)
		c.Set(contextRoleKey, role)
		c.Next()
	}
}

// RequireRole chặn request nếu role trong context thấp hơn min (403). Phải đặt
// SAU RequireWorkspace trong chuỗi middleware.
func RequireRole(min string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, ok := RoleFromContext(c)
		if !ok || !RoleAtLeast(role, min) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "error": "forbidden"})
			return
		}
		c.Next()
	}
}

// WorkspaceIDFromContext lấy workspace đã validate; ok=false nếu chưa qua
// RequireWorkspace.
func WorkspaceIDFromContext(c *gin.Context) (string, bool) {
	v, ok := c.Get(contextWorkspaceIDKey)
	if !ok {
		return "", false
	}
	id, ok := v.(string)
	return id, ok
}

// RoleFromContext lấy role đã resolve; ok=false nếu chưa qua RequireWorkspace.
func RoleFromContext(c *gin.Context) (string, bool) {
	v, ok := c.Get(contextRoleKey)
	if !ok {
		return "", false
	}
	role, ok := v.(string)
	return role, ok
}
