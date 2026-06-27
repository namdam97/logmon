package domain

import "time"

// Membership liên kết một user với một workspace kèm role RBAC. Là entity của
// identity BC — bất biến, đổi role trả bản sao mới (WithRole).
type Membership struct {
	workspaceID WorkspaceID
	userID      UserID
	role        Role
	joinedAt    time.Time
}

// NewMembership tạo membership mới đã validate.
func NewMembership(workspaceID WorkspaceID, userID UserID, role Role, now time.Time) (Membership, error) {
	if !role.Valid() {
		return Membership{}, newValidationError("role", "invalid role")
	}
	if now.IsZero() {
		return Membership{}, newValidationError("joinedAt", "must be set")
	}
	return Membership{workspaceID: workspaceID, userID: userID, role: role, joinedAt: now}, nil
}

// ReconstructMembership dựng lại từ storage — KHÔNG validate lại.
func ReconstructMembership(workspaceID WorkspaceID, userID UserID, role Role, joinedAt time.Time) Membership {
	return Membership{workspaceID: workspaceID, userID: userID, role: role, joinedAt: joinedAt}
}

// WorkspaceID trả về workspace của membership.
func (m Membership) WorkspaceID() WorkspaceID { return m.workspaceID }

// UserID trả về user của membership.
func (m Membership) UserID() UserID { return m.userID }

// Role trả về vai trò RBAC hiện tại.
func (m Membership) Role() Role { return m.role }

// JoinedAt trả về thời điểm tham gia workspace (UTC).
func (m Membership) JoinedAt() time.Time { return m.joinedAt }

// WithRole trả về bản sao membership với role mới (bất biến). Trả
// ValidationError nếu role không hợp lệ.
func (m Membership) WithRole(role Role) (Membership, error) {
	if !role.Valid() {
		return Membership{}, newValidationError("role", "invalid role")
	}
	cp := m
	cp.role = role
	return cp, nil
}
