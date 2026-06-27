package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func init() { gin.SetMode(gin.TestMode) }

type fakeResolver struct {
	role string
	ok   bool
	err  error
}

func (f fakeResolver) Resolve(context.Context, string, string) (string, bool, error) {
	return f.role, f.ok, f.err
}

func TestRoleAtLeast(t *testing.T) {
	require.True(t, RoleAtLeast(RoleAdmin, RoleEditor))
	require.True(t, RoleAtLeast(RoleEditor, RoleEditor))
	require.False(t, RoleAtLeast(RoleViewer, RoleEditor))
	require.False(t, RoleAtLeast("garbage", RoleViewer))
	require.True(t, RoleAtLeast(RolePlatformAdmin, RoleAdmin))
}

// withUser gắn sẵn userID vào context (giả lập đã qua RequireAuth).
func withUser(userID string) gin.HandlerFunc {
	return func(c *gin.Context) { c.Set(contextUserIDKey, userID); c.Next() }
}

func runWorkspace(t *testing.T, resolver MembershipResolver, header string, mws ...gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	r := gin.New()
	chain := append([]gin.HandlerFunc{withUser("u-1"), RequireWorkspace(resolver)}, mws...)
	chain = append(chain, func(c *gin.Context) {
		ws, _ := WorkspaceIDFromContext(c)
		role, _ := RoleFromContext(c)
		c.JSON(http.StatusOK, gin.H{"ws": ws, "role": role})
	})
	r.GET("/x", chain...)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	if header != "" {
		req.Header.Set(WorkspaceHeader, header)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestRequireWorkspace(t *testing.T) {
	t.Run("missing header → 400", func(t *testing.T) {
		w := runWorkspace(t, fakeResolver{role: "admin", ok: true}, "")
		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("not a member → 404", func(t *testing.T) {
		w := runWorkspace(t, fakeResolver{ok: false}, "ws-1")
		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("infra error → 500", func(t *testing.T) {
		w := runWorkspace(t, fakeResolver{err: context.DeadlineExceeded}, "ws-1")
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("member → sets context", func(t *testing.T) {
		w := runWorkspace(t, fakeResolver{role: "editor", ok: true}, "ws-1")
		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), `"ws":"ws-1"`)
		require.Contains(t, w.Body.String(), `"role":"editor"`)
	})
}

func TestRequireRole(t *testing.T) {
	t.Run("sufficient role → pass", func(t *testing.T) {
		w := runWorkspace(t, fakeResolver{role: "admin", ok: true}, "ws-1", RequireRole(RoleEditor))
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("insufficient role → 403", func(t *testing.T) {
		w := runWorkspace(t, fakeResolver{role: "viewer", ok: true}, "ws-1", RequireRole(RoleAdmin))
		require.Equal(t, http.StatusForbidden, w.Code)
	})
}
