package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/user/domain"
)

func validRefreshInput() domain.NewRefreshTokenInput {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return domain.NewRefreshTokenInput{
		ID:        "id-1",
		UserID:    "user-1",
		FamilyID:  "fam-1",
		TokenHash: "abcdef",
		ExpiresAt: now.Add(14 * 24 * time.Hour),
		CreatedAt: now,
	}
}

func TestNewRefreshToken_Valid(t *testing.T) {
	rt, err := domain.NewRefreshToken(validRefreshInput())

	require.NoError(t, err)
	require.Equal(t, "user-1", rt.UserID())
	require.Equal(t, "fam-1", rt.FamilyID())
	require.Equal(t, "abcdef", rt.TokenHash())
	require.False(t, rt.IsUsed(), "token mới chưa dùng")
}

func TestNewRefreshToken_RejectsInvalid(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*domain.NewRefreshTokenInput)
		field  string
	}{
		{name: "empty id", mutate: func(in *domain.NewRefreshTokenInput) { in.ID = "" }, field: "id"},
		{name: "empty userID", mutate: func(in *domain.NewRefreshTokenInput) { in.UserID = "" }, field: "userID"},
		{name: "empty familyID", mutate: func(in *domain.NewRefreshTokenInput) { in.FamilyID = "" }, field: "familyID"},
		{name: "empty tokenHash", mutate: func(in *domain.NewRefreshTokenInput) { in.TokenHash = "" }, field: "tokenHash"},
		{
			name:   "expiresAt not after createdAt",
			mutate: func(in *domain.NewRefreshTokenInput) { in.ExpiresAt = in.CreatedAt },
			field:  "expiresAt",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validRefreshInput()
			tt.mutate(&in)

			_, err := domain.NewRefreshToken(in)

			require.Error(t, err)
			var ve *domain.ValidationError
			require.True(t, errors.As(err, &ve))
			require.Equal(t, tt.field, ve.Field)
		})
	}
}

func TestRefreshToken_IsExpired(t *testing.T) {
	rt, err := domain.NewRefreshToken(validRefreshInput())
	require.NoError(t, err)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	require.False(t, rt.IsExpired(now))
	require.True(t, rt.IsExpired(now.Add(15*24*time.Hour)))
}

func TestReconstructRefreshToken_PreservesUsedAt(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	used := now.Add(time.Hour)

	rt := domain.ReconstructRefreshToken("id", "u", "f", "h", &used, now.Add(time.Hour*2), now)

	require.True(t, rt.IsUsed())
	require.Equal(t, "f", rt.FamilyID())
}
