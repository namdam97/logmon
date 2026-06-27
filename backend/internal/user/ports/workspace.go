package ports

import (
	"context"

	"github.com/namdam97/logmon/backend/internal/user/domain"
)

// WorkspaceRepository lưu trữ workspace (write side).
type WorkspaceRepository interface {
	// Save lưu workspace mới. Trả domain.ErrSlugTaken nếu slug đã tồn tại.
	Save(ctx context.Context, w domain.Workspace) error
	// ByID lấy workspace theo id. Trả domain.ErrWorkspaceNotFound nếu không có.
	ByID(ctx context.Context, id domain.WorkspaceID) (domain.Workspace, error)
}

// WorkspaceReader đọc danh sách workspace của một user (read side).
type WorkspaceReader interface {
	// ListForUser trả các workspace mà user là thành viên.
	ListForUser(ctx context.Context, userID domain.UserID) ([]domain.Workspace, error)
}

// MembershipRepository quản lý thành viên workspace (write side).
type MembershipRepository interface {
	// Save thêm thành viên. Trả domain.ErrMembershipExists nếu đã là thành viên.
	Save(ctx context.Context, m domain.Membership) error
	// UpdateRole đổi role của thành viên. Trả domain.ErrNotMember nếu chưa là thành viên.
	UpdateRole(ctx context.Context, workspaceID domain.WorkspaceID, userID domain.UserID, role domain.Role) error
	// Remove xóa thành viên khỏi workspace. Trả domain.ErrNotMember nếu chưa là thành viên.
	Remove(ctx context.Context, workspaceID domain.WorkspaceID, userID domain.UserID) error
}

// MembershipReader đọc thông tin thành viên (read side + RBAC resolve).
type MembershipReader interface {
	// ByWorkspaceAndUser lấy membership; trả domain.ErrNotMember nếu không có.
	ByWorkspaceAndUser(ctx context.Context, workspaceID domain.WorkspaceID, userID domain.UserID) (domain.Membership, error)
	// ListByWorkspace liệt kê thành viên của workspace.
	ListByWorkspace(ctx context.Context, workspaceID domain.WorkspaceID) ([]domain.Membership, error)
	// CountAdmins đếm số thành viên có role >= admin (cho guard last-admin).
	CountAdmins(ctx context.Context, workspaceID domain.WorkspaceID) (int, error)
}
