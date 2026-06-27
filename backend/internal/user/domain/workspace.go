package domain

import (
	"strings"
	"time"
)

const (
	_maxWorkspaceName = 100
	_maxSlugLength    = 100
)

// WorkspaceID là value object định danh workspace (UUID dạng chuỗi).
type WorkspaceID struct {
	value string
}

// NewWorkspaceID validate và bọc một định danh workspace không rỗng.
func NewWorkspaceID(raw string) (WorkspaceID, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return WorkspaceID{}, newValidationError("workspaceId", "must not be empty")
	}
	return WorkspaceID{value: v}, nil
}

// String trả về biểu diễn chuỗi của WorkspaceID.
func (id WorkspaceID) String() string { return id.value }

// Workspace là aggregate root cho tenancy. Field không export — bất biến, chỉ
// tạo qua NewWorkspace/ReconstructWorkspace. slug dùng làm namespace ES data
// stream nên phải an toàn DNS (lowercase, alnum, hyphen).
type Workspace struct {
	id        WorkspaceID
	name      string
	slug      string
	createdAt time.Time
	updatedAt time.Time
}

// NewWorkspace tạo workspace mới đã validate. slug rỗng → tự sinh từ name.
func NewWorkspace(id WorkspaceID, name, slug string, now time.Time) (Workspace, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Workspace{}, newValidationError("name", "must not be empty")
	}
	if len(name) > _maxWorkspaceName {
		return Workspace{}, newValidationError("name", "too long")
	}
	if now.IsZero() {
		return Workspace{}, newValidationError("createdAt", "must be set")
	}
	s := strings.TrimSpace(slug)
	if s == "" {
		s = Slugify(name)
	} else {
		s = Slugify(s)
	}
	if s == "" {
		return Workspace{}, newValidationError("slug", "could not derive a valid slug")
	}
	return Workspace{id: id, name: name, slug: s, createdAt: now, updatedAt: now}, nil
}

// ReconstructWorkspace dựng lại từ storage — KHÔNG validate lại (dữ liệu đã sạch).
func ReconstructWorkspace(id WorkspaceID, name, slug string, createdAt, updatedAt time.Time) Workspace {
	return Workspace{id: id, name: name, slug: slug, createdAt: createdAt, updatedAt: updatedAt}
}

// ID trả về định danh workspace.
func (w Workspace) ID() WorkspaceID { return w.id }

// Name trả về tên hiển thị.
func (w Workspace) Name() string { return w.name }

// Slug trả về slug (namespace ES).
func (w Workspace) Slug() string { return w.slug }

// CreatedAt trả về thời điểm tạo (UTC).
func (w Workspace) CreatedAt() time.Time { return w.createdAt }

// UpdatedAt trả về thời điểm cập nhật gần nhất (UTC).
func (w Workspace) UpdatedAt() time.Time { return w.updatedAt }

// Slugify chuyển chuỗi tự do thành slug an toàn: lowercase, ký tự không phải
// [a-z0-9] thành '-', gộp '-' liên tiếp, cắt '-' đầu/cuối, giới hạn độ dài.
func Slugify(raw string) string {
	var b strings.Builder
	prevDash := true // chặn '-' dẫn đầu
	for _, r := range strings.ToLower(strings.TrimSpace(raw)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	s := strings.TrimRight(b.String(), "-")
	if len(s) > _maxSlugLength {
		s = strings.TrimRight(s[:_maxSlugLength], "-")
	}
	return s
}
