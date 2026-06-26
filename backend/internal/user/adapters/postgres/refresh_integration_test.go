//go:build integration

// Integration test cho RefreshRepository — cần Postgres thật (DATABASE_URL) đã áp
// migrations (gồm 000005_refresh_tokens). Chạy: make test-integration.
package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/user/adapters/postgres"
	"github.com/namdam97/logmon/backend/internal/user/domain"
)

func newRefreshPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dburl := os.Getenv("DATABASE_URL")
	if dburl == "" {
		t.Skip("DATABASE_URL chưa set — bỏ qua integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dburl)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	// CASCADE: xoá refresh_tokens trước (FK users), rồi users test.
	_, err = pool.Exec(context.Background(), "TRUNCATE refresh_tokens, users RESTART IDENTITY CASCADE")
	require.NoError(t, err)
	return pool
}

// seedUserRow chèn một user để thoả FK refresh_tokens.user_id, trả về userID.
func seedUserRow(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	id := uuid.NewString()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, email, password_hash, created_at) VALUES ($1, $2, $3, now())`,
		id, id+"@logmon.local", "hash")
	require.NoError(t, err)
	return id
}

func newRefreshToken(t *testing.T, userID, familyID, hash string, expiresAt time.Time) domain.RefreshToken {
	t.Helper()
	rt, err := domain.NewRefreshToken(domain.NewRefreshTokenInput{
		ID:        uuid.NewString(),
		UserID:    userID,
		FamilyID:  familyID,
		TokenHash: hash,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)
	return rt
}

func TestRefreshRepository_InsertAndByHash(t *testing.T) {
	pool := newRefreshPool(t)
	ctx := context.Background()
	repo := postgres.NewRefreshRepository(pool)
	userID := seedUserRow(t, pool)

	rt := newRefreshToken(t, userID, uuid.NewString(), "hash-1", time.Now().Add(time.Hour))
	require.NoError(t, repo.Insert(ctx, rt))

	got, err := repo.ByHash(ctx, "hash-1")
	require.NoError(t, err)
	require.Equal(t, userID, got.UserID())
	require.False(t, got.IsUsed())
}

func TestRefreshRepository_ByHashMissing(t *testing.T) {
	pool := newRefreshPool(t)
	repo := postgres.NewRefreshRepository(pool)

	_, err := repo.ByHash(context.Background(), "nope")
	require.ErrorIs(t, err, domain.ErrRefreshTokenInvalid)
}

func TestRefreshRepository_ClaimByHashIsAtomicOnce(t *testing.T) {
	pool := newRefreshPool(t)
	ctx := context.Background()
	repo := postgres.NewRefreshRepository(pool)
	userID := seedUserRow(t, pool)
	now := time.Now().UTC()
	require.NoError(t, repo.Insert(ctx, newRefreshToken(t, userID, uuid.NewString(), "hash-2", now.Add(time.Hour))))

	claimed, ok, err := repo.ClaimByHash(ctx, "hash-2", now)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, userID, claimed.UserID())

	// Claim lần 2 thất bại (đã dùng) — đây là cốt lõi chống reuse.
	_, ok2, err := repo.ClaimByHash(ctx, "hash-2", now)
	require.NoError(t, err)
	require.False(t, ok2)

	reused, err := repo.ByHash(ctx, "hash-2")
	require.NoError(t, err)
	require.True(t, reused.IsUsed())
}

func TestRefreshRepository_ClaimByHashRejectsExpired(t *testing.T) {
	pool := newRefreshPool(t)
	ctx := context.Background()
	repo := postgres.NewRefreshRepository(pool)
	userID := seedUserRow(t, pool)
	now := time.Now().UTC()
	require.NoError(t, repo.Insert(ctx, newRefreshToken(t, userID, uuid.NewString(), "hash-exp", now.Add(time.Minute))))

	_, ok, err := repo.ClaimByHash(ctx, "hash-exp", now.Add(time.Hour))
	require.NoError(t, err)
	require.False(t, ok, "token hết hạn không claim được")
}

func TestRefreshRepository_RevokeFamily(t *testing.T) {
	pool := newRefreshPool(t)
	ctx := context.Background()
	repo := postgres.NewRefreshRepository(pool)
	userID := seedUserRow(t, pool)
	family := uuid.NewString()
	now := time.Now().UTC()
	require.NoError(t, repo.Insert(ctx, newRefreshToken(t, userID, family, "hash-a", now.Add(time.Hour))))
	require.NoError(t, repo.Insert(ctx, newRefreshToken(t, userID, family, "hash-b", now.Add(time.Hour))))

	require.NoError(t, repo.RevokeFamily(ctx, family))

	_, err := repo.ByHash(ctx, "hash-a")
	require.ErrorIs(t, err, domain.ErrRefreshTokenInvalid)
	_, err = repo.ByHash(ctx, "hash-b")
	require.ErrorIs(t, err, domain.ErrRefreshTokenInvalid)
}
