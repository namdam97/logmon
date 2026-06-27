package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/namdam97/logmon/backend/internal/user/domain"
	"github.com/namdam97/logmon/backend/internal/user/ports"
)

// WorkspaceRepository lưu trữ workspace trên PostgreSQL. Mọi query parameterized.
type WorkspaceRepository struct {
	pool *pgxpool.Pool
}

// MembershipRepository lưu trữ thành viên workspace. Tách struct riêng vì cả
// workspace lẫn membership đều cần method Save (tránh đụng tên trên cùng struct).
type MembershipRepository struct {
	pool *pgxpool.Pool
}

// Verify compliance tại compile time.
var (
	_ ports.WorkspaceRepository  = (*WorkspaceRepository)(nil)
	_ ports.WorkspaceReader      = (*WorkspaceRepository)(nil)
	_ ports.MembershipRepository = (*MembershipRepository)(nil)
	_ ports.MembershipReader     = (*MembershipRepository)(nil)
)

// NewWorkspaceRepository tạo repository workspace với pool đã khởi tạo.
func NewWorkspaceRepository(pool *pgxpool.Pool) *WorkspaceRepository {
	return &WorkspaceRepository{pool: pool}
}

// NewMembershipRepository tạo repository membership với pool đã khởi tạo.
func NewMembershipRepository(pool *pgxpool.Pool) *MembershipRepository {
	return &MembershipRepository{pool: pool}
}

// Save chèn workspace mới. Trả domain.ErrSlugTaken khi vi phạm unique slug.
func (r *WorkspaceRepository) Save(ctx context.Context, w domain.Workspace) error {
	const q = `INSERT INTO workspaces (id, name, slug, created_at, updated_at)
	           VALUES ($1, $2, $3, $4, $5)`
	_, err := r.pool.Exec(ctx, q, w.ID().String(), w.Name(), w.Slug(), w.CreatedAt(), w.UpdatedAt())
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode {
			return domain.ErrSlugTaken
		}
		return fmt.Errorf("insert workspace: %w", err)
	}
	return nil
}

// ByID lấy workspace theo id. Trả domain.ErrWorkspaceNotFound nếu không có.
func (r *WorkspaceRepository) ByID(ctx context.Context, id domain.WorkspaceID) (domain.Workspace, error) {
	const q = `SELECT id, name, slug, created_at, updated_at FROM workspaces WHERE id = $1`
	row := r.pool.QueryRow(ctx, q, id.String())
	return scanWorkspace(row)
}

// ListForUser trả các workspace mà user là thành viên (JOIN membership).
func (r *WorkspaceRepository) ListForUser(ctx context.Context, userID domain.UserID) ([]domain.Workspace, error) {
	const q = `SELECT w.id, w.name, w.slug, w.created_at, w.updated_at
	           FROM workspaces w
	           JOIN workspace_members m ON m.workspace_id = w.id
	           WHERE m.user_id = $1
	           ORDER BY w.created_at`
	rows, err := r.pool.Query(ctx, q, userID.String())
	if err != nil {
		return nil, fmt.Errorf("query workspaces for user: %w", err)
	}
	defer rows.Close()

	var out []domain.Workspace
	for rows.Next() {
		w, err := scanWorkspace(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspaces: %w", err)
	}
	return out, nil
}

// Save thêm thành viên. Trả domain.ErrMembershipExists nếu trùng PK.
func (r *MembershipRepository) Save(ctx context.Context, m domain.Membership) error {
	const q = `INSERT INTO workspace_members (workspace_id, user_id, role, joined_at)
	           VALUES ($1, $2, $3, $4)`
	_, err := r.pool.Exec(ctx, q, m.WorkspaceID().String(), m.UserID().String(), m.Role().String(), m.JoinedAt())
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode {
			return domain.ErrMembershipExists
		}
		return fmt.Errorf("insert membership: %w", err)
	}
	return nil
}

// UpdateRole đổi role thành viên. Trả domain.ErrNotMember nếu không có dòng.
func (r *MembershipRepository) UpdateRole(ctx context.Context, wid domain.WorkspaceID, uid domain.UserID, role domain.Role) error {
	const q = `UPDATE workspace_members SET role = $1 WHERE workspace_id = $2 AND user_id = $3`
	tag, err := r.pool.Exec(ctx, q, role.String(), wid.String(), uid.String())
	if err != nil {
		return fmt.Errorf("update member role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotMember
	}
	return nil
}

// Remove xóa thành viên. Trả domain.ErrNotMember nếu không có dòng.
func (r *MembershipRepository) Remove(ctx context.Context, wid domain.WorkspaceID, uid domain.UserID) error {
	const q = `DELETE FROM workspace_members WHERE workspace_id = $1 AND user_id = $2`
	tag, err := r.pool.Exec(ctx, q, wid.String(), uid.String())
	if err != nil {
		return fmt.Errorf("delete member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotMember
	}
	return nil
}

// ByWorkspaceAndUser lấy membership; trả domain.ErrNotMember nếu không có.
func (r *MembershipRepository) ByWorkspaceAndUser(ctx context.Context, wid domain.WorkspaceID, uid domain.UserID) (domain.Membership, error) {
	const q = `SELECT workspace_id, user_id, role, joined_at
	           FROM workspace_members WHERE workspace_id = $1 AND user_id = $2`
	row := r.pool.QueryRow(ctx, q, wid.String(), uid.String())
	return scanMembership(row)
}

// ListByWorkspace liệt kê thành viên của workspace.
func (r *MembershipRepository) ListByWorkspace(ctx context.Context, wid domain.WorkspaceID) ([]domain.Membership, error) {
	const q = `SELECT workspace_id, user_id, role, joined_at
	           FROM workspace_members WHERE workspace_id = $1 ORDER BY joined_at`
	rows, err := r.pool.Query(ctx, q, wid.String())
	if err != nil {
		return nil, fmt.Errorf("query members: %w", err)
	}
	defer rows.Close()

	var out []domain.Membership
	for rows.Next() {
		m, err := scanMembership(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate members: %w", err)
	}
	return out, nil
}

// CountAdmins đếm thành viên có role admin trở lên (cho guard last-admin).
func (r *MembershipRepository) CountAdmins(ctx context.Context, wid domain.WorkspaceID) (int, error) {
	const q = `SELECT count(*) FROM workspace_members
	           WHERE workspace_id = $1 AND role IN ('admin','platform_admin')`
	var n int
	if err := r.pool.QueryRow(ctx, q, wid.String()).Scan(&n); err != nil {
		return 0, fmt.Errorf("count admins: %w", err)
	}
	return n, nil
}

func scanWorkspace(row pgx.Row) (domain.Workspace, error) {
	var id, name, slug string
	var createdAt, updatedAt time.Time
	if err := row.Scan(&id, &name, &slug, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Workspace{}, domain.ErrWorkspaceNotFound
		}
		return domain.Workspace{}, fmt.Errorf("scan workspace: %w", err)
	}
	wid, err := domain.NewWorkspaceID(id)
	if err != nil {
		return domain.Workspace{}, err
	}
	return domain.ReconstructWorkspace(wid, name, slug, createdAt, updatedAt), nil
}

func scanMembership(row pgx.Row) (domain.Membership, error) {
	var wid, uid, role string
	var joinedAt time.Time
	if err := row.Scan(&wid, &uid, &role, &joinedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Membership{}, domain.ErrNotMember
		}
		return domain.Membership{}, fmt.Errorf("scan membership: %w", err)
	}
	w, err := domain.NewWorkspaceID(wid)
	if err != nil {
		return domain.Membership{}, err
	}
	u, err := domain.NewUserID(uid)
	if err != nil {
		return domain.Membership{}, err
	}
	parsed, err := domain.ParseRole(role)
	if err != nil {
		return domain.Membership{}, err
	}
	return domain.ReconstructMembership(w, u, parsed, joinedAt), nil
}
