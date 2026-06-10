package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/user/domain"
)

func TestNewEmail(t *testing.T) {
	tests := []struct {
		name    string
		give    string
		want    string
		wantErr bool
	}{
		{name: "valid lowercase", give: "user@example.com", want: "user@example.com"},
		{name: "normalises case and spaces", give: "  User@Example.COM ", want: "user@example.com"},
		{name: "empty rejected", give: "   ", wantErr: true},
		{name: "missing at sign rejected", give: "userexample.com", wantErr: true},
		{name: "missing domain dot rejected", give: "user@examplecom", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			email, err := domain.NewEmail(tt.give)

			if tt.wantErr {
				require.Error(t, err)
				var ve *domain.ValidationError
				require.True(t, errors.As(err, &ve))
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, email.String())
		})
	}
}

func TestNewUserID(t *testing.T) {
	tests := []struct {
		name    string
		give    string
		wantErr bool
	}{
		{name: "valid id", give: "01HXYZ", wantErr: false},
		{name: "empty rejected", give: "  ", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := domain.NewUserID(tt.give)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotEmpty(t, id.String())
		})
	}
}

func TestNewUser(t *testing.T) {
	email, err := domain.NewEmail("user@example.com")
	require.NoError(t, err)
	id, err := domain.NewUserID("user-1")
	require.NoError(t, err)
	now := time.Now().UTC()

	tests := []struct {
		name     string
		giveHash string
		giveTime time.Time
		wantErr  bool
	}{
		{name: "valid user", giveHash: "$2a$10$hash", giveTime: now},
		{name: "empty hash rejected", giveHash: "", giveTime: now, wantErr: true},
		{name: "zero createdAt rejected", giveHash: "$2a$10$hash", giveTime: time.Time{}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := domain.NewUser(id, email, tt.giveHash, tt.giveTime)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "user@example.com", u.Email().String())
			require.Equal(t, tt.giveHash, u.PasswordHash())
			require.Equal(t, "user-1", u.ID().String())
		})
	}
}
