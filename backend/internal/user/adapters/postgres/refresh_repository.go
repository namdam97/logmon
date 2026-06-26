package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/namdam97/logmon/backend/internal/user/domain"
	"github.com/namdam97/logmon/backend/internal/user/ports"
)

// RefreshRepository lưu refresh token trong PostgreSQL. Mọi query parameterized.
type RefreshRepository struct {
	pool *pgxpool.Pool
}

var _ ports.RefreshTokenRepository = (*RefreshRepository)(nil)

// NewRefreshRepository tạo repository với connection pool.
func NewRefreshRepository(pool *pgxpool.Pool) *RefreshRepository {
	return &RefreshRepository{pool: pool}
}

// Insert lưu một refresh token mới (used_at = NULL).
func (r *RefreshRepository) Insert(ctx context.Context, t domain.RefreshToken) error {
	const q = `INSERT INTO refresh_tokens (id, user_id, family_id, token_hash, expires_at, created_at)
	           VALUES ($1, $2, $3, $4, $5, $6)`
	_, err := r.pool.Exec(ctx, q,
		t.ID(), t.UserID(), t.FamilyID(), t.TokenHash(), t.ExpiresAt(), t.CreatedAt())
	if err != nil {
		return fmt.Errorf("insert refresh token: %w", err)
	}
	return nil
}

// ClaimByHash đánh dấu used_at nguyên tử cho token chưa dùng & chưa hết hạn. Câu
// UPDATE ... RETURNING bảo đảm chỉ một request claim được token (an toàn đồng thời).
func (r *RefreshRepository) ClaimByHash(ctx context.Context, hash string, now time.Time) (domain.RefreshToken, bool, error) {
	const q = `UPDATE refresh_tokens
	           SET used_at = $2
	           WHERE token_hash = $1 AND used_at IS NULL AND expires_at > $2
	           RETURNING id, user_id, family_id, token_hash, used_at, expires_at, created_at`
	row := r.pool.QueryRow(ctx, q, hash, now)
	t, err := scanRefresh(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.RefreshToken{}, false, nil
		}
		return domain.RefreshToken{}, false, fmt.Errorf("claim refresh token: %w", err)
	}
	return t, true, nil
}

// ByHash lấy token theo hash bất kể trạng thái. ErrRefreshTokenInvalid nếu không có.
func (r *RefreshRepository) ByHash(ctx context.Context, hash string) (domain.RefreshToken, error) {
	const q = `SELECT id, user_id, family_id, token_hash, used_at, expires_at, created_at
	           FROM refresh_tokens WHERE token_hash = $1`
	row := r.pool.QueryRow(ctx, q, hash)
	t, err := scanRefresh(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.RefreshToken{}, domain.ErrRefreshTokenInvalid
		}
		return domain.RefreshToken{}, fmt.Errorf("get refresh token: %w", err)
	}
	return t, nil
}

// RevokeFamily xoá toàn bộ token thuộc một family (reuse detection / logout).
func (r *RefreshRepository) RevokeFamily(ctx context.Context, familyID string) error {
	const q = `DELETE FROM refresh_tokens WHERE family_id = $1`
	if _, err := r.pool.Exec(ctx, q, familyID); err != nil {
		return fmt.Errorf("revoke refresh family: %w", err)
	}
	return nil
}

func scanRefresh(row pgx.Row) (domain.RefreshToken, error) {
	var id, userID, familyID, tokenHash string
	var usedAt *time.Time
	var expiresAt, createdAt time.Time
	if err := row.Scan(&id, &userID, &familyID, &tokenHash, &usedAt, &expiresAt, &createdAt); err != nil {
		return domain.RefreshToken{}, err
	}
	return domain.ReconstructRefreshToken(id, userID, familyID, tokenHash, usedAt, expiresAt, createdAt), nil
}
