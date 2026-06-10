package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/user/app"
	"github.com/namdam97/logmon/backend/internal/user/domain"
)

// --- test doubles (inject dependencies, không dùng mutable globals) ---

type fakeRepo struct {
	byID    map[string]domain.User
	byEmail map[string]domain.User
	saveErr error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{byID: map[string]domain.User{}, byEmail: map[string]domain.User{}}
}

func (r *fakeRepo) Save(_ context.Context, u domain.User) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	r.byID[u.ID().String()] = u
	r.byEmail[u.Email().String()] = u
	return nil
}

func (r *fakeRepo) ByID(_ context.Context, id domain.UserID) (domain.User, error) {
	u, ok := r.byID[id.String()]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound
	}
	return u, nil
}

func (r *fakeRepo) ByEmail(_ context.Context, email domain.Email) (domain.User, error) {
	u, ok := r.byEmail[email.String()]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound
	}
	return u, nil
}

type fakeHasher struct{}

func (fakeHasher) Hash(plain string) (string, error) { return "hashed:" + plain, nil }

func (fakeHasher) Verify(hash, plain string) error {
	if hash == "hashed:"+plain {
		return nil
	}
	return errors.New("mismatch")
}

type fixedIDGen struct{ id string }

func (g fixedIDGen) NewID() string { return g.id }

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

type fakeTokens struct{ token string }

func (f fakeTokens) Issue(string) (string, error) { return f.token, nil }

func newService(repo *fakeRepo) *app.Service {
	return app.NewService(
		repo,
		fakeHasher{},
		fixedIDGen{id: "user-1"},
		fixedClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		fakeTokens{token: "tok-123"},
	)
}

func TestServiceRegister(t *testing.T) {
	tests := []struct {
		name    string
		give    app.RegisterInput
		wantErr bool
	}{
		{name: "valid registration", give: app.RegisterInput{Email: "a@b.com", Password: "password123"}},
		{name: "invalid email", give: app.RegisterInput{Email: "nope", Password: "password123"}, wantErr: true},
		{name: "short password", give: app.RegisterInput{Email: "a@b.com", Password: "short"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			svc := newService(repo)

			user, err := svc.Register(context.Background(), tt.give)

			if tt.wantErr {
				require.Error(t, err)
				var ve *domain.ValidationError
				require.True(t, errors.As(err, &ve))
				return
			}
			require.NoError(t, err)
			require.Equal(t, "a@b.com", user.Email().String())
			require.Equal(t, "hashed:password123", user.PasswordHash())
			require.Equal(t, "user-1", user.ID().String())
		})
	}
}

func TestServiceGet(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo)
	created, err := svc.Register(context.Background(), app.RegisterInput{Email: "a@b.com", Password: "password123"})
	require.NoError(t, err)

	t.Run("returns existing user", func(t *testing.T) {
		got, err := svc.Get(context.Background(), created.ID().String())
		require.NoError(t, err)
		require.Equal(t, created.ID().String(), got.ID().String())
	})

	t.Run("not found maps to ErrUserNotFound", func(t *testing.T) {
		_, err := svc.Get(context.Background(), "missing")
		require.Error(t, err)
		require.True(t, errors.Is(err, domain.ErrUserNotFound))
	})

	t.Run("invalid id is validation error", func(t *testing.T) {
		_, err := svc.Get(context.Background(), "  ")
		require.Error(t, err)
		var ve *domain.ValidationError
		require.True(t, errors.As(err, &ve))
	})
}

func TestServiceLogin(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo)
	_, err := svc.Register(context.Background(), app.RegisterInput{Email: "a@b.com", Password: "password123"})
	require.NoError(t, err)

	tests := []struct {
		name      string
		give      app.LoginInput
		wantErr   bool
		wantToken string
	}{
		{name: "valid login", give: app.LoginInput{Email: "a@b.com", Password: "password123"}, wantToken: "tok-123"},
		{name: "wrong password", give: app.LoginInput{Email: "a@b.com", Password: "nope"}, wantErr: true},
		{name: "unknown email", give: app.LoginInput{Email: "x@y.com", Password: "password123"}, wantErr: true},
		{name: "malformed email", give: app.LoginInput{Email: "bad", Password: "password123"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, token, err := svc.Login(context.Background(), tt.give)
			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, domain.ErrInvalidCredentials)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantToken, token)
			require.Equal(t, "a@b.com", user.Email().String())
		})
	}
}
