// Package postgres implement ports.UserRepository trên PostgreSQL qua pgx/v5.
// Mọi query đều parameterized ($1, $2) — KHÔNG string concatenation.
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

const uniqueViolationCode = "23505"

// Repository lưu trữ user trong PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// Verify compliance tại compile time.
var _ ports.UserRepository = (*Repository)(nil)

// NewRepository tạo Repository với connection pool đã khởi tạo.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Save chèn user mới. Trả về domain.ErrEmailTaken khi vi phạm unique trên email.
func (r *Repository) Save(ctx context.Context, u domain.User) error {
	const q = `INSERT INTO users (id, email, password_hash, created_at)
	           VALUES ($1, $2, $3, $4)`
	_, err := r.pool.Exec(ctx, q,
		u.ID().String(), u.Email().String(), u.PasswordHash(), u.CreatedAt())
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode {
			return domain.ErrEmailTaken
		}
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

// ByID lấy user theo id. Trả về domain.ErrUserNotFound nếu không có dòng nào.
func (r *Repository) ByID(ctx context.Context, id domain.UserID) (domain.User, error) {
	const q = `SELECT id, email, password_hash, created_at
	           FROM users WHERE id = $1`
	row := r.pool.QueryRow(ctx, q, id.String())

	var rawID, rawEmail, hash string
	var createdAt time.Time
	if err := row.Scan(&rawID, &rawEmail, &hash, &createdAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, domain.ErrUserNotFound
		}
		return domain.User{}, fmt.Errorf("scan user: %w", err)
	}
	return reconstruct(rawID, rawEmail, hash, createdAt)
}
