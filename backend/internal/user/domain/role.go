package domain

import "strings"

// Role là vai trò RBAC của một thành viên trong workspace. Phân cấp tăng dần
// (viewer ⊂ editor ⊂ admin ⊂ platform_admin) — doc_v2/09 §2. So sánh bằng
// AtLeast theo thứ tự số, KHÔNG hardcode danh sách quyền tại nơi gọi.
type Role int

// Enum bắt đầu từ 1 (zero value không hợp lệ — tránh viewer mặc định ngầm).
const (
	// RoleViewer: chỉ đọc resource trong workspace + log search.
	RoleViewer Role = iota + 1
	// RoleEditor: + CRUD rules/SLO/incidents, ack/silence, timeline.
	RoleEditor
	// RoleAdmin: + quản lý members, channels, pipeline mode/ILM, oncall.
	RoleAdmin
	// RolePlatformAdmin: mọi workspace + tạo/xóa workspace.
	RolePlatformAdmin
)

// AtLeast báo role hiện tại có quyền tối thiểu bằng min hay không (phân cấp).
func (r Role) AtLeast(min Role) bool { return r >= min }

// String trả về biểu diễn chuỗi dùng ở API/DB.
func (r Role) String() string {
	switch r {
	case RoleViewer:
		return "viewer"
	case RoleEditor:
		return "editor"
	case RoleAdmin:
		return "admin"
	case RolePlatformAdmin:
		return "platform_admin"
	default:
		return "unknown"
	}
}

// Valid báo role có nằm trong tập hợp lệ không.
func (r Role) Valid() bool { return r >= RoleViewer && r <= RolePlatformAdmin }

// ParseRole chuyển chuỗi thành Role (case-insensitive). Trả ValidationError nếu
// không hợp lệ.
func ParseRole(raw string) (Role, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "viewer":
		return RoleViewer, nil
	case "editor":
		return RoleEditor, nil
	case "admin":
		return RoleAdmin, nil
	case "platform_admin":
		return RolePlatformAdmin, nil
	default:
		return 0, newValidationError("role", "must be one of viewer|editor|admin|platform_admin")
	}
}
