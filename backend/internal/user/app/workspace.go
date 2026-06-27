// Package app chứa use case của identity BC. workspace.go xử lý vòng đời
// workspace + thành viên (multi-tenancy GĐ3.6). Mọi thao tác chỉ phụ thuộc
// interfaces ở ports (DIP).
package app

import (
	"context"
	"fmt"

	"github.com/namdam97/logmon/backend/internal/user/domain"
	"github.com/namdam97/logmon/backend/internal/user/ports"
)

// WorkspaceService xử lý tạo/liệt kê workspace. Khi tạo, người tạo trở thành
// admin của workspace mới.
type WorkspaceService struct {
	workspaces ports.WorkspaceRepository
	wsReader   ports.WorkspaceReader
	members    ports.MembershipRepository
	ids        ports.IDGenerator
	clock      ports.Clock
}

// NewWorkspaceService tạo service với dependencies inject.
func NewWorkspaceService(
	workspaces ports.WorkspaceRepository,
	wsReader ports.WorkspaceReader,
	members ports.MembershipRepository,
	ids ports.IDGenerator,
	clock ports.Clock,
) *WorkspaceService {
	return &WorkspaceService{workspaces: workspaces, wsReader: wsReader, members: members, ids: ids, clock: clock}
}

// CreateWorkspaceInput là dữ liệu tạo workspace mới.
type CreateWorkspaceInput struct {
	Name        string
	Slug        string // tùy chọn — rỗng thì sinh từ Name
	OwnerUserID string // user tạo workspace → admin đầu tiên
}

// Create tạo workspace + thêm owner làm admin (atomic ở tầng adapter qua tx nếu
// cần; ở đây tuần tự — slug-taken chặn trước khi tạo membership).
func (s *WorkspaceService) Create(ctx context.Context, in CreateWorkspaceInput) (domain.Workspace, error) {
	ownerID, err := domain.NewUserID(in.OwnerUserID)
	if err != nil {
		return domain.Workspace{}, err
	}
	wid, err := domain.NewWorkspaceID(s.ids.NewID())
	if err != nil {
		return domain.Workspace{}, err
	}
	now := s.clock.Now()
	ws, err := domain.NewWorkspace(wid, in.Name, in.Slug, now)
	if err != nil {
		return domain.Workspace{}, err
	}
	if err := s.workspaces.Save(ctx, ws); err != nil {
		return domain.Workspace{}, fmt.Errorf("save workspace: %w", err)
	}
	owner, err := domain.NewMembership(wid, ownerID, domain.RoleAdmin, now)
	if err != nil {
		return domain.Workspace{}, err
	}
	if err := s.members.Save(ctx, owner); err != nil {
		return domain.Workspace{}, fmt.Errorf("add owner membership: %w", err)
	}
	return ws, nil
}

// ListForUser trả các workspace mà user là thành viên.
func (s *WorkspaceService) ListForUser(ctx context.Context, userID string) ([]domain.Workspace, error) {
	uid, err := domain.NewUserID(userID)
	if err != nil {
		return nil, err
	}
	return s.wsReader.ListForUser(ctx, uid)
}
