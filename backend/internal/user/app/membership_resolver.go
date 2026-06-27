package app

import (
	"context"
	"errors"

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

// Resolve trả role (chuỗi) của user trong workspace. ok=false nghĩa là không
// phải thành viên (hoặc input không hợp lệ) → middleware map sang 404 để không
// lộ tồn tại; err!=nil chỉ dành cho lỗi hạ tầng (middleware map sang 500).
func (r *MembershipResolver) Resolve(ctx context.Context, userID, workspaceID string) (string, bool, error) {
	wid, err := domain.NewWorkspaceID(workspaceID)
	if err != nil {
		return "", false, nil
	}
	uid, err := domain.NewUserID(userID)
	if err != nil {
		return "", false, nil
	}
	m, err := r.memReader.ByWorkspaceAndUser(ctx, wid, uid)
	if err != nil {
		if errors.Is(err, domain.ErrNotMember) {
			return "", false, nil
		}
		return "", false, err
	}
	return m.Role().String(), true, nil
}
