package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/user/app"
	"github.com/namdam97/logmon/backend/internal/user/domain"
)

// ---- fakes (workspace + membership) ----

type fakeWorkspaceStore struct {
	byID    map[string]domain.Workspace
	bySlug  map[string]domain.Workspace
	forUser map[string][]domain.Workspace
	saveErr error
}

func newFakeWorkspaceStore() *fakeWorkspaceStore {
	return &fakeWorkspaceStore{
		byID:    map[string]domain.Workspace{},
		bySlug:  map[string]domain.Workspace{},
		forUser: map[string][]domain.Workspace{},
	}
}

func (s *fakeWorkspaceStore) Save(_ context.Context, w domain.Workspace) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	if _, ok := s.bySlug[w.Slug()]; ok {
		return domain.ErrSlugTaken
	}
	s.byID[w.ID().String()] = w
	s.bySlug[w.Slug()] = w
	return nil
}

func (s *fakeWorkspaceStore) ByID(_ context.Context, id domain.WorkspaceID) (domain.Workspace, error) {
	w, ok := s.byID[id.String()]
	if !ok {
		return domain.Workspace{}, domain.ErrWorkspaceNotFound
	}
	return w, nil
}

func (s *fakeWorkspaceStore) ListForUser(_ context.Context, userID domain.UserID) ([]domain.Workspace, error) {
	return s.forUser[userID.String()], nil
}

type fakeMemberStore struct {
	// key = workspaceID|userID
	byKey   map[string]domain.Membership
	saveErr error
}

func newFakeMemberStore() *fakeMemberStore {
	return &fakeMemberStore{byKey: map[string]domain.Membership{}}
}

func memKey(wid domain.WorkspaceID, uid domain.UserID) string {
	return wid.String() + "|" + uid.String()
}

func (s *fakeMemberStore) Save(_ context.Context, m domain.Membership) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	k := memKey(m.WorkspaceID(), m.UserID())
	if _, ok := s.byKey[k]; ok {
		return domain.ErrMembershipExists
	}
	s.byKey[k] = m
	return nil
}

func (s *fakeMemberStore) UpdateRole(_ context.Context, wid domain.WorkspaceID, uid domain.UserID, role domain.Role) error {
	k := memKey(wid, uid)
	m, ok := s.byKey[k]
	if !ok {
		return domain.ErrNotMember
	}
	updated, _ := m.WithRole(role)
	s.byKey[k] = updated
	return nil
}

func (s *fakeMemberStore) Remove(_ context.Context, wid domain.WorkspaceID, uid domain.UserID) error {
	k := memKey(wid, uid)
	if _, ok := s.byKey[k]; !ok {
		return domain.ErrNotMember
	}
	delete(s.byKey, k)
	return nil
}

func (s *fakeMemberStore) ByWorkspaceAndUser(_ context.Context, wid domain.WorkspaceID, uid domain.UserID) (domain.Membership, error) {
	m, ok := s.byKey[memKey(wid, uid)]
	if !ok {
		return domain.Membership{}, domain.ErrNotMember
	}
	return m, nil
}

func (s *fakeMemberStore) ListByWorkspace(_ context.Context, wid domain.WorkspaceID) ([]domain.Membership, error) {
	var out []domain.Membership
	for _, m := range s.byKey {
		if m.WorkspaceID().String() == wid.String() {
			out = append(out, m)
		}
	}
	return out, nil
}

func (s *fakeMemberStore) CountAdmins(_ context.Context, wid domain.WorkspaceID) (int, error) {
	n := 0
	for _, m := range s.byKey {
		if m.WorkspaceID().String() == wid.String() && m.Role().AtLeast(domain.RoleAdmin) {
			n++
		}
	}
	return n, nil
}

// ---- tests ----

func TestCreateWorkspaceAddsOwnerAsAdmin(t *testing.T) {
	ws := newFakeWorkspaceStore()
	mem := newFakeMemberStore()
	clock := fixedClock{t: time.Unix(1000, 0).UTC()}
	svc := app.NewWorkspaceService(ws, ws, mem, fixedIDGen{id: "ws-1"}, clock)

	w, err := svc.Create(context.Background(), app.CreateWorkspaceInput{Name: "Acme Corp", OwnerUserID: "u-1"})
	require.NoError(t, err)
	require.Equal(t, "acme-corp", w.Slug())

	wid, _ := domain.NewWorkspaceID("ws-1")
	uid, _ := domain.NewUserID("u-1")
	owner, err := mem.ByWorkspaceAndUser(context.Background(), wid, uid)
	require.NoError(t, err)
	require.Equal(t, domain.RoleAdmin, owner.Role())
}

func TestAddMemberByEmail(t *testing.T) {
	users := newFakeRepo()
	uid, _ := domain.NewUserID("u-2")
	email, _ := domain.NewEmail("bob@example.com")
	u, _ := domain.NewUser(uid, email, "hashed:x", time.Unix(1, 0).UTC())
	require.NoError(t, users.Save(context.Background(), u))

	mem := newFakeMemberStore()
	svc := app.NewMemberService(mem, mem, users, fixedClock{t: time.Unix(2000, 0).UTC()})

	m, err := svc.Add(context.Background(), app.AddMemberInput{WorkspaceID: "ws-1", Email: "bob@example.com", Role: "editor"})
	require.NoError(t, err)
	require.Equal(t, domain.RoleEditor, m.Role())
	require.Equal(t, "u-2", m.UserID().String())
}

func TestAddMemberUnknownEmail(t *testing.T) {
	mem := newFakeMemberStore()
	svc := app.NewMemberService(mem, mem, newFakeRepo(), fixedClock{t: time.Unix(2000, 0).UTC()})
	_, err := svc.Add(context.Background(), app.AddMemberInput{WorkspaceID: "ws-1", Email: "ghost@example.com", Role: "viewer"})
	require.ErrorIs(t, err, domain.ErrUserNotFound)
}

func TestUpdateRoleGuardsLastAdmin(t *testing.T) {
	mem := newFakeMemberStore()
	wid, _ := domain.NewWorkspaceID("ws-1")
	uid, _ := domain.NewUserID("u-1")
	admin, _ := domain.NewMembership(wid, uid, domain.RoleAdmin, time.Unix(1, 0).UTC())
	require.NoError(t, mem.Save(context.Background(), admin))

	svc := app.NewMemberService(mem, mem, newFakeRepo(), fixedClock{t: time.Unix(2, 0).UTC()})

	// Hạ admin cuối cùng → chặn.
	_, err := svc.UpdateRole(context.Background(), "ws-1", "u-1", "viewer")
	require.ErrorIs(t, err, domain.ErrLastAdmin)

	// Thêm admin thứ 2 → giờ hạ quyền người đầu được.
	uid2, _ := domain.NewUserID("u-2")
	admin2, _ := domain.NewMembership(wid, uid2, domain.RoleAdmin, time.Unix(1, 0).UTC())
	require.NoError(t, mem.Save(context.Background(), admin2))
	got, err := svc.UpdateRole(context.Background(), "ws-1", "u-1", "viewer")
	require.NoError(t, err)
	require.Equal(t, domain.RoleViewer, got.Role())
}

func TestRemoveMemberGuardsLastAdmin(t *testing.T) {
	mem := newFakeMemberStore()
	wid, _ := domain.NewWorkspaceID("ws-1")
	uid, _ := domain.NewUserID("u-1")
	admin, _ := domain.NewMembership(wid, uid, domain.RoleAdmin, time.Unix(1, 0).UTC())
	require.NoError(t, mem.Save(context.Background(), admin))

	svc := app.NewMemberService(mem, mem, newFakeRepo(), fixedClock{t: time.Unix(2, 0).UTC()})
	err := svc.Remove(context.Background(), "ws-1", "u-1")
	require.ErrorIs(t, err, domain.ErrLastAdmin)
}

func TestMembershipResolver(t *testing.T) {
	mem := newFakeMemberStore()
	wid, _ := domain.NewWorkspaceID("ws-1")
	uid, _ := domain.NewUserID("u-1")
	m, _ := domain.NewMembership(wid, uid, domain.RoleEditor, time.Unix(1, 0).UTC())
	require.NoError(t, mem.Save(context.Background(), m))

	r := app.NewMembershipResolver(mem)
	role, ok, err := r.Resolve(context.Background(), "u-1", "ws-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "editor", role)

	_, ok, err = r.Resolve(context.Background(), "ghost", "ws-1")
	require.NoError(t, err)
	require.False(t, ok)
}
