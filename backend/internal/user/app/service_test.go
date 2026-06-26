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
	byID        map[string]domain.User
	byEmail     map[string]domain.User
	saveErr     error
	updatedHash map[string]string
	updateErr   error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		byID:        map[string]domain.User{},
		byEmail:     map[string]domain.User{},
		updatedHash: map[string]string{},
	}
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

func (r *fakeRepo) UpdatePasswordHash(_ context.Context, id domain.UserID, hash string) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	r.updatedHash[id.String()] = hash
	return nil
}

type fakeHasher struct{ rehash bool }

func (fakeHasher) Hash(plain string) (string, error) { return "hashed:" + plain, nil }

func (fakeHasher) Verify(hash, plain string) error {
	if hash == "hashed:"+plain {
		return nil
	}
	return errors.New("mismatch")
}

func (h fakeHasher) NeedsRehash(string) bool { return h.rehash }

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

func newServiceWith(repo *fakeRepo, hasher fakeHasher) *app.Service {
	return app.NewService(
		repo, hasher, fixedIDGen{id: "user-1"},
		fixedClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		fakeTokens{token: "tok-123"},
	)
}

func TestServiceLoginLazyRehash(t *testing.T) {
	repo := newFakeRepo()
	svc := newServiceWith(repo, fakeHasher{rehash: true})
	_, err := svc.Register(context.Background(), app.RegisterInput{Email: "a@b.com", Password: "password123"})
	require.NoError(t, err)

	_, _, err = svc.Login(context.Background(), app.LoginInput{Email: "a@b.com", Password: "password123"})

	require.NoError(t, err)
	require.Equal(t, "hashed:password123", repo.updatedHash["user-1"],
		"login thành công phải re-hash mật khẩu khi hash cũ cần nâng cấp")
}

func TestServiceLoginNoRehashWhenCurrent(t *testing.T) {
	repo := newFakeRepo()
	svc := newServiceWith(repo, fakeHasher{rehash: false})
	_, err := svc.Register(context.Background(), app.RegisterInput{Email: "a@b.com", Password: "password123"})
	require.NoError(t, err)

	_, _, err = svc.Login(context.Background(), app.LoginInput{Email: "a@b.com", Password: "password123"})

	require.NoError(t, err)
	require.Empty(t, repo.updatedHash, "hash hiện hành thì không re-hash")
}

func TestServiceLoginRehashFailureDoesNotBlock(t *testing.T) {
	repo := newFakeRepo()
	repo.updateErr = errors.New("db down")
	svc := newServiceWith(repo, fakeHasher{rehash: true})
	_, err := svc.Register(context.Background(), app.RegisterInput{Email: "a@b.com", Password: "password123"})
	require.NoError(t, err)

	_, token, err := svc.Login(context.Background(), app.LoginInput{Email: "a@b.com", Password: "password123"})

	require.NoError(t, err, "re-hash lỗi vẫn cho login thành công (best-effort)")
	require.Equal(t, "tok-123", token)
}
