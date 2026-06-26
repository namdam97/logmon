package app_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/user/app"
	"github.com/namdam97/logmon/backend/internal/user/domain"
)

// --- test doubles cho refresh ports ---

type fakeRefreshRepo struct {
	byHash          map[string]domain.RefreshToken
	revokedFamilies []string
	insertErr       error
}

func newFakeRefreshRepo() *fakeRefreshRepo {
	return &fakeRefreshRepo{byHash: map[string]domain.RefreshToken{}}
}

func (r *fakeRefreshRepo) Insert(_ context.Context, t domain.RefreshToken) error {
	if r.insertErr != nil {
		return r.insertErr
	}
	r.byHash[t.TokenHash()] = t
	return nil
}

func (r *fakeRefreshRepo) ClaimByHash(_ context.Context, hash string, now time.Time) (domain.RefreshToken, bool, error) {
	t, ok := r.byHash[hash]
	if !ok || t.IsUsed() || t.IsExpired(now) {
		return domain.RefreshToken{}, false, nil
	}
	used := now
	claimed := domain.ReconstructRefreshToken(
		t.ID(), t.UserID(), t.FamilyID(), t.TokenHash(), &used, t.ExpiresAt(), t.CreatedAt())
	r.byHash[hash] = claimed
	return claimed, true, nil
}

func (r *fakeRefreshRepo) ByHash(_ context.Context, hash string) (domain.RefreshToken, error) {
	t, ok := r.byHash[hash]
	if !ok {
		return domain.RefreshToken{}, domain.ErrRefreshTokenInvalid
	}
	return t, nil
}

func (r *fakeRefreshRepo) RevokeFamily(_ context.Context, familyID string) error {
	r.revokedFamilies = append(r.revokedFamilies, familyID)
	for h, t := range r.byHash {
		if t.FamilyID() == familyID {
			delete(r.byHash, h)
		}
	}
	return nil
}

type fakeCodec struct{ n int }

func (c *fakeCodec) Generate() (string, error) {
	c.n++
	return fmt.Sprintf("raw-%d", c.n), nil
}

func (fakeCodec) Hash(raw string) string { return "hash:" + raw }

func newRefreshSvc(repo *fakeRefreshRepo) *app.RefreshService {
	return app.NewRefreshService(
		repo, &fakeCodec{}, fakeTokens{token: "tok-123"},
		fixedIDGen{id: "fam-1"},
		fixedClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		time.Hour, nil,
	)
}

func TestRefreshIssue_PersistsUnusedToken(t *testing.T) {
	repo := newFakeRefreshRepo()
	svc := newRefreshSvc(repo)

	raw, err := svc.Issue(context.Background(), "user-1")

	require.NoError(t, err)
	require.NotEmpty(t, raw)
	require.Len(t, repo.byHash, 1)
	stored := repo.byHash["hash:"+raw]
	require.Equal(t, "user-1", stored.UserID())
	require.False(t, stored.IsUsed())
}

func TestRefreshRotate_IssuesNewPairAndMarksOldUsed(t *testing.T) {
	repo := newFakeRefreshRepo()
	svc := newRefreshSvc(repo)
	raw1, err := svc.Issue(context.Background(), "user-1")
	require.NoError(t, err)

	pair, err := svc.Rotate(context.Background(), raw1)

	require.NoError(t, err)
	require.Equal(t, "tok-123", pair.Access)
	require.NotEmpty(t, pair.Refresh)
	require.NotEqual(t, raw1, pair.Refresh, "refresh token phải đổi sau rotate")
	require.True(t, repo.byHash["hash:"+raw1].IsUsed(), "token cũ phải đánh dấu đã dùng")
	require.False(t, repo.byHash["hash:"+pair.Refresh].IsUsed())
}

func TestRefreshRotate_ReuseRevokesFamily(t *testing.T) {
	repo := newFakeRefreshRepo()
	svc := newRefreshSvc(repo)
	raw1, err := svc.Issue(context.Background(), "user-1")
	require.NoError(t, err)
	_, err = svc.Rotate(context.Background(), raw1) // raw1 giờ đã dùng
	require.NoError(t, err)

	_, err = svc.Rotate(context.Background(), raw1) // dùng lại → reuse

	require.ErrorIs(t, err, domain.ErrRefreshTokenReused)
	require.Contains(t, repo.revokedFamilies, "fam-1")
	require.Empty(t, repo.byHash, "reuse phải thu hồi toàn bộ family")
}

func TestRefreshRotate_InvalidToken(t *testing.T) {
	repo := newFakeRefreshRepo()
	svc := newRefreshSvc(repo)

	_, err := svc.Rotate(context.Background(), "never-issued")

	require.ErrorIs(t, err, domain.ErrRefreshTokenInvalid)
}

func TestRefreshRotate_ExpiredTokenIsInvalid(t *testing.T) {
	repo := newFakeRefreshRepo()
	svc := newRefreshSvc(repo)
	past := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	repo.byHash["hash:expired"] = domain.ReconstructRefreshToken(
		"id", "user-1", "fam-1", "hash:expired", nil, past.Add(time.Hour), past)

	_, err := svc.Rotate(context.Background(), "expired")

	require.ErrorIs(t, err, domain.ErrRefreshTokenInvalid)
}

func TestRefreshRevoke_RemovesFamily(t *testing.T) {
	repo := newFakeRefreshRepo()
	svc := newRefreshSvc(repo)
	raw1, err := svc.Issue(context.Background(), "user-1")
	require.NoError(t, err)

	require.NoError(t, svc.Revoke(context.Background(), raw1))

	require.Contains(t, repo.revokedFamilies, "fam-1")
	_, err = svc.Rotate(context.Background(), raw1)
	require.ErrorIs(t, err, domain.ErrRefreshTokenInvalid)
}

func TestRefreshRevoke_UnknownTokenNoError(t *testing.T) {
	repo := newFakeRefreshRepo()
	svc := newRefreshSvc(repo)

	require.NoError(t, svc.Revoke(context.Background(), "nope"))
}
