package app

import (
	"context"
	"fmt"

	"github.com/namdam97/logmon/backend/internal/user/domain"
	"github.com/namdam97/logmon/backend/internal/user/ports"
)

// MemberService quản lý thành viên workspace: liệt kê, mời (theo email), đổi
// role, xóa. Có guard không cho hạ quyền/xóa admin cuối cùng.
type MemberService struct {
	members   ports.MembershipRepository
	memReader ports.MembershipReader
	users     ports.UserRepository
	clock     ports.Clock
}

// NewMemberService tạo service với dependencies inject.
func NewMemberService(
	members ports.MembershipRepository,
	memReader ports.MembershipReader,
	users ports.UserRepository,
	clock ports.Clock,
) *MemberService {
	return &MemberService{members: members, memReader: memReader, users: users, clock: clock}
}

// List trả danh sách thành viên của workspace.
func (s *MemberService) List(ctx context.Context, workspaceID string) ([]domain.Membership, error) {
	wid, err := domain.NewWorkspaceID(workspaceID)
	if err != nil {
		return nil, err
	}
	return s.memReader.ListByWorkspace(ctx, wid)
}

// AddMemberInput là dữ liệu mời thành viên (theo email của user đã tồn tại).
type AddMemberInput struct {
	WorkspaceID string
	Email       string
	Role        string
}

// Add mời một user (đã tồn tại) vào workspace với role chỉ định.
func (s *MemberService) Add(ctx context.Context, in AddMemberInput) (domain.Membership, error) {
	wid, err := domain.NewWorkspaceID(in.WorkspaceID)
	if err != nil {
		return domain.Membership{}, err
	}
	role, err := domain.ParseRole(in.Role)
	if err != nil {
		return domain.Membership{}, err
	}
	email, err := domain.NewEmail(in.Email)
	if err != nil {
		return domain.Membership{}, err
	}
	u, err := s.users.ByEmail(ctx, email)
	if err != nil {
		return domain.Membership{}, err // ErrUserNotFound → 404
	}
	m, err := domain.NewMembership(wid, u.ID(), role, s.clock.Now())
	if err != nil {
		return domain.Membership{}, err
	}
	if err := s.members.Save(ctx, m); err != nil {
		return domain.Membership{}, fmt.Errorf("save membership: %w", err)
	}
	return m, nil
}

// UpdateRole đổi role thành viên. Chặn hạ quyền admin cuối cùng (ErrLastAdmin).
func (s *MemberService) UpdateRole(ctx context.Context, workspaceID, userID, role string) (domain.Membership, error) {
	wid, uid, err := parseWorkspaceUser(workspaceID, userID)
	if err != nil {
		return domain.Membership{}, err
	}
	newRole, err := domain.ParseRole(role)
	if err != nil {
		return domain.Membership{}, err
	}
	current, err := s.memReader.ByWorkspaceAndUser(ctx, wid, uid)
	if err != nil {
		return domain.Membership{}, err // ErrNotMember → 404
	}
	if err := s.guardLastAdmin(ctx, wid, current.Role(), newRole); err != nil {
		return domain.Membership{}, err
	}
	updated, err := current.WithRole(newRole)
	if err != nil {
		return domain.Membership{}, err
	}
	if err := s.members.UpdateRole(ctx, wid, uid, newRole); err != nil {
		return domain.Membership{}, fmt.Errorf("update role: %w", err)
	}
	return updated, nil
}

// Remove xóa thành viên. Chặn xóa admin cuối cùng (ErrLastAdmin).
func (s *MemberService) Remove(ctx context.Context, workspaceID, userID string) error {
	wid, uid, err := parseWorkspaceUser(workspaceID, userID)
	if err != nil {
		return err
	}
	current, err := s.memReader.ByWorkspaceAndUser(ctx, wid, uid)
	if err != nil {
		return err // ErrNotMember → 404
	}
	// Xóa = hạ quyền xuống "không còn quyền" → áp guard nếu đang là admin.
	if current.Role().AtLeast(domain.RoleAdmin) {
		count, err := s.memReader.CountAdmins(ctx, wid)
		if err != nil {
			return fmt.Errorf("count admins: %w", err)
		}
		if count <= 1 {
			return domain.ErrLastAdmin
		}
	}
	if err := s.members.Remove(ctx, wid, uid); err != nil {
		return fmt.Errorf("remove member: %w", err)
	}
	return nil
}

// guardLastAdmin chặn việc hạ một admin xuống dưới admin khi đó là admin cuối.
func (s *MemberService) guardLastAdmin(ctx context.Context, wid domain.WorkspaceID, cur, next domain.Role) error {
	if cur.AtLeast(domain.RoleAdmin) && !next.AtLeast(domain.RoleAdmin) {
		count, err := s.memReader.CountAdmins(ctx, wid)
		if err != nil {
			return fmt.Errorf("count admins: %w", err)
		}
		if count <= 1 {
			return domain.ErrLastAdmin
		}
	}
	return nil
}

func parseWorkspaceUser(workspaceID, userID string) (domain.WorkspaceID, domain.UserID, error) {
	wid, err := domain.NewWorkspaceID(workspaceID)
	if err != nil {
		return domain.WorkspaceID{}, domain.UserID{}, err
	}
	uid, err := domain.NewUserID(userID)
	if err != nil {
		return domain.WorkspaceID{}, domain.UserID{}, err
	}
	return wid, uid, nil
}
