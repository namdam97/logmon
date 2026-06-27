package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseRoleAndHierarchy(t *testing.T) {
	tests := []struct {
		give    string
		want    Role
		wantErr bool
	}{
		{give: "viewer", want: RoleViewer},
		{give: "EDITOR", want: RoleEditor},
		{give: " admin ", want: RoleAdmin},
		{give: "platform_admin", want: RolePlatformAdmin},
		{give: "root", wantErr: true},
		{give: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.give, func(t *testing.T) {
			got, err := ParseRole(tt.give)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
			require.Equal(t, tt.want.String(), got.String())
		})
	}
}

func TestRoleAtLeast(t *testing.T) {
	require.True(t, RoleAdmin.AtLeast(RoleEditor))
	require.True(t, RoleEditor.AtLeast(RoleEditor))
	require.False(t, RoleViewer.AtLeast(RoleEditor))
	require.True(t, RolePlatformAdmin.AtLeast(RoleAdmin))
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		give string
		want string
	}{
		{give: "Acme Corp", want: "acme-corp"},
		{give: "  Hello,  World!  ", want: "hello-world"},
		{give: "Team_42", want: "team-42"},
		{give: "---weird---", want: "weird"},
		{give: "ALLCAPS", want: "allcaps"},
		{give: "!!!", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.give, func(t *testing.T) {
			require.Equal(t, tt.want, Slugify(tt.give))
		})
	}
}

func TestNewWorkspace(t *testing.T) {
	id, err := NewWorkspaceID("ws-1")
	require.NoError(t, err)
	now := time.Unix(1000, 0).UTC()

	t.Run("derives slug from name", func(t *testing.T) {
		w, err := NewWorkspace(id, "Acme Corp", "", now)
		require.NoError(t, err)
		require.Equal(t, "acme-corp", w.Slug())
		require.Equal(t, "Acme Corp", w.Name())
		require.Equal(t, now, w.CreatedAt())
	})

	t.Run("normalizes provided slug", func(t *testing.T) {
		w, err := NewWorkspace(id, "Acme", "My Slug", now)
		require.NoError(t, err)
		require.Equal(t, "my-slug", w.Slug())
	})

	t.Run("rejects empty name", func(t *testing.T) {
		_, err := NewWorkspace(id, "   ", "", now)
		require.Error(t, err)
	})

	t.Run("rejects name that slugifies to empty with no slug", func(t *testing.T) {
		_, err := NewWorkspace(id, "!!!", "", now)
		require.Error(t, err)
	})
}

func TestMembershipWithRole(t *testing.T) {
	wid, _ := NewWorkspaceID("ws-1")
	uid, _ := NewUserID("u-1")
	now := time.Unix(2000, 0).UTC()

	m, err := NewMembership(wid, uid, RoleViewer, now)
	require.NoError(t, err)
	require.Equal(t, RoleViewer, m.Role())

	upgraded, err := m.WithRole(RoleAdmin)
	require.NoError(t, err)
	require.Equal(t, RoleAdmin, upgraded.Role())
	// bất biến: bản gốc không đổi
	require.Equal(t, RoleViewer, m.Role())

	_, err = m.WithRole(Role(99))
	require.Error(t, err)
}
