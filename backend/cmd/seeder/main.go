// Command seeder nạp dữ liệu demo nhất quán cho local dev. Idempotent: user đã
// tồn tại thì bỏ qua (không lỗi). Chạy qua `make seed`.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/namdam97/logmon/backend/internal/shared/logger"
	userpg "github.com/namdam97/logmon/backend/internal/user/adapters/postgres"
	usersys "github.com/namdam97/logmon/backend/internal/user/adapters/system"
	"github.com/namdam97/logmon/backend/internal/user/domain"
)

const _connectTimeout = 10 * time.Second

// seedUser là một bản ghi demo. Mật khẩu chỉ cho local dev.
type seedUser struct {
	email    string
	password string
}

var _seedUsers = []seedUser{
	{email: "admin@logmon.local", password: "password123"},
	{email: "dev@logmon.local", password: "password123"},
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "seeder:", err)
		os.Exit(1)
	}
}

func run() error {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return errors.New("DATABASE_URL not configured")
	}
	log := logger.New(os.Stdout, "info")

	ctx, cancel := context.WithTimeout(context.Background(), _connectTimeout)
	defer cancel()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer pool.Close()

	repo := userpg.NewRepository(pool)
	hasher := usersys.NewBcryptHasher(0)
	ids := usersys.NewUUIDGenerator()
	clock := usersys.NewClock()

	for _, su := range _seedUsers {
		if err := seedOne(ctx, repo, hasher, ids, clock, log, su); err != nil {
			return err
		}
	}
	log.Info(ctx, "seed complete")
	return nil
}

func seedOne(
	ctx context.Context,
	repo *userpg.Repository,
	hasher *usersys.BcryptHasher,
	ids *usersys.UUIDGenerator,
	clock *usersys.Clock,
	log *logger.Logger,
	su seedUser,
) error {
	email, err := domain.NewEmail(su.email)
	if err != nil {
		return fmt.Errorf("seed email %q: %w", su.email, err)
	}
	hash, err := hasher.Hash(su.password)
	if err != nil {
		return fmt.Errorf("seed hash: %w", err)
	}
	id, err := domain.NewUserID(ids.NewID())
	if err != nil {
		return fmt.Errorf("seed id: %w", err)
	}
	user, err := domain.NewUser(id, email, hash, clock.Now())
	if err != nil {
		return fmt.Errorf("seed user: %w", err)
	}

	switch err := repo.Save(ctx, user); {
	case err == nil:
		log.Infof(ctx, "seeded user", "email", su.email)
	case errors.Is(err, domain.ErrEmailTaken):
		log.Infof(ctx, "user exists, skip", "email", su.email)
	default:
		return fmt.Errorf("save %q: %w", su.email, err)
	}
	return nil
}
