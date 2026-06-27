package app

import (
	"context"

	"github.com/namdam97/logmon/backend/internal/user/domain"
	"github.com/namdam97/logmon/backend/internal/user/ports"
)

// MembershipResolver giải quyết role của user trong một workspace — dùng bởi
// middleware tenancy (shared/auth) để validate membership + nạp role vào context.
// Trả role dạng chuỗi để shared kernel không phụ thuộc domain identity.
type MembershipResolver struct {
	memReader ports.MembershipReader
}

// NewMembershipResolver tạo resolver.
func NewMembershipResolver(memReader ports.MembershipReader) *MembershipResolver {
	return &MembershipResolver{memReader: memReader}
}

// Resolve trả role (chuỗi) của user trong workspace. Trả domain.ErrNotMember nếu
// user không thuộc workspace (middleware map sang 404).
func (r *MembershipResolver) Resolve(ctx context.Context, userID, workspaceID string) (string, error) {
	wid, err := domain.NewWorkspaceID(workspaceID)
	if err != nil {
		return "", err
	}
	uid, err := domain.NewUserID(userID)
	if err != nil {
		return "", err
	}
	m, err := r.memReader.ByWorkspaceAndUser(ctx, wid, uid)
	if err != nil {
		return "", err
	}
	return m.Role().String(), nil
}
