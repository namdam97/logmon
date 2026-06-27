package http_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	httpadapter "github.com/namdam97/logmon/backend/internal/user/adapters/http"
	"github.com/namdam97/logmon/backend/internal/user/app"
	"github.com/namdam97/logmon/backend/internal/user/domain"
)

// ---- fakes ----

type fakeWorkspaceSvc struct {
	created domain.Workspace
	listed  []domain.Workspace
	err     error
}

func (f fakeWorkspaceSvc) Create(context.Context, app.CreateWorkspaceInput) (domain.Workspace, error) {
	return f.created, f.err
}
func (f fakeWorkspaceSvc) ListForUser(context.Context, string) ([]domain.Workspace, error) {
	return f.listed, f.err
}

type fakeMemberSvc struct {
	member  domain.Membership
	members []domain.Membership
	err     error
}

func (f fakeMemberSvc) List(context.Context, string) ([]domain.Membership, error) {
	return f.members, f.err
}
func (f fakeMemberSvc) Add(context.Context, app.AddMemberInput) (domain.Membership, error) {
	return f.member, f.err
}
func (f fakeMemberSvc) UpdateRole(context.Context, string, string, string) (domain.Membership, error) {
	return f.member, f.err
}
func (f fakeMemberSvc) Remove(context.Context, string, string) error { return f.err }

type fakeResolver struct {
	role string
	ok   bool
}

func (f fakeResolver) Resolve(context.Context, string, string) (string, bool, error) {
	return f.role, f.ok, nil
}

func mkWorkspace(t *testing.T) domain.Workspace {
	t.Helper()
	id, err := domain.NewWorkspaceID("ws-1")
	require.NoError(t, err)
	w, err := domain.NewWorkspace(id, "Acme", "", time.Unix(1, 0).UTC())
	require.NoError(t, err)
	return w
}

func mkMember(t *testing.T, role domain.Role) domain.Membership {
	t.Helper()
	wid, _ := domain.NewWorkspaceID("ws-1")
	uid, _ := domain.NewUserID("u-2")
	m, err := domain.NewMembership(wid, uid, role, time.Unix(1, 0).UTC())
	require.NoError(t, err)
	return m
}

// stub authMW sets the authenticated user id key used by auth.UserIDFromContext.
func stubAuth(c *gin.Context) { c.Set("auth_user_id", "u-1"); c.Next() }

func wsRouter(ws *fakeWorkspaceSvc, mem *fakeMemberSvc, res fakeResolver) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := httpadapter.NewWorkspaceHandler(ws, mem, res, nil)
	h.Register(r.Group("/api/v1"), stubAuth)
	return r
}

func do(t *testing.T, r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// ---- tests ----

func TestListWorkspaces(t *testing.T) {
	r := wsRouter(&fakeWorkspaceSvc{listed: []domain.Workspace{mkWorkspace(t)}}, &fakeMemberSvc{}, fakeResolver{})
	w := do(t, r, http.MethodGet, "/api/v1/workspaces", "")
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "acme")
}

func TestCreateWorkspace(t *testing.T) {
	r := wsRouter(&fakeWorkspaceSvc{created: mkWorkspace(t)}, &fakeMemberSvc{}, fakeResolver{})
	w := do(t, r, http.MethodPost, "/api/v1/workspaces", `{"name":"Acme"}`)
	require.Equal(t, http.StatusCreated, w.Code)
}

func TestListMembersRequiresMembership(t *testing.T) {
	// not a member → 404
	r := wsRouter(&fakeWorkspaceSvc{}, &fakeMemberSvc{}, fakeResolver{ok: false})
	w := do(t, r, http.MethodGet, "/api/v1/workspaces/ws-1/members", "")
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestListMembersViewerOK(t *testing.T) {
	r := wsRouter(&fakeWorkspaceSvc{}, &fakeMemberSvc{members: []domain.Membership{mkMember(t, domain.RoleEditor)}}, fakeResolver{role: "viewer", ok: true})
	w := do(t, r, http.MethodGet, "/api/v1/workspaces/ws-1/members", "")
	require.Equal(t, http.StatusOK, w.Code)
}

func TestAddMemberRequiresAdmin(t *testing.T) {
	// editor tries to add member → 403
	r := wsRouter(&fakeWorkspaceSvc{}, &fakeMemberSvc{member: mkMember(t, domain.RoleEditor)}, fakeResolver{role: "editor", ok: true})
	w := do(t, r, http.MethodPost, "/api/v1/workspaces/ws-1/members", `{"email":"b@c.d","role":"editor"}`)
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestAddMemberAdminOK(t *testing.T) {
	r := wsRouter(&fakeWorkspaceSvc{}, &fakeMemberSvc{member: mkMember(t, domain.RoleEditor)}, fakeResolver{role: "admin", ok: true})
	w := do(t, r, http.MethodPost, "/api/v1/workspaces/ws-1/members", `{"email":"b@c.d","role":"editor"}`)
	require.Equal(t, http.StatusCreated, w.Code)
}

func TestUpdateAndRemoveMember(t *testing.T) {
	r := wsRouter(&fakeWorkspaceSvc{}, &fakeMemberSvc{member: mkMember(t, domain.RoleAdmin)}, fakeResolver{role: "admin", ok: true})
	w := do(t, r, http.MethodPut, "/api/v1/workspaces/ws-1/members/u-2", `{"role":"admin"}`)
	require.Equal(t, http.StatusOK, w.Code)

	w = do(t, r, http.MethodDelete, "/api/v1/workspaces/ws-1/members/u-2", "")
	require.Equal(t, http.StatusOK, w.Code)
}
