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

// _defaultWorkspaceID là workspace mặc định (seed ở migration 000011). Mọi user
// demo được gán làm thành viên để login + X-Workspace-ID hoạt động ngay.
const _defaultWorkspaceID = "00000000-0000-0000-0000-000000000001"

// seedUser là một bản ghi demo. Mật khẩu chỉ cho local dev.
type seedUser struct {
	email    string
	password string
	role     domain.Role
}

var _seedUsers = []seedUser{
	{email: "admin@logmon.local", password: "password123", role: domain.RoleAdmin},
	{email: "dev@logmon.local", password: "password123", role: domain.RoleEditor},
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
	members := userpg.NewMembershipRepository(pool)
	hasher := usersys.NewArgon2idHasher()
	ids := usersys.NewUUIDGenerator()
	clock := usersys.NewClock()

	for _, su := range _seedUsers {
		if err := seedOne(ctx, repo, members, hasher, ids, clock, log, su); err != nil {
			return err
		}
	}
	log.Info(ctx, "seed complete")
	return nil
}

func seedOne(
	ctx context.Context,
	repo *userpg.Repository,
	members *userpg.MembershipRepository,
	hasher *usersys.Argon2idHasher,
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
		// Lấy id user hiện hữu để gán membership.
		existing, getErr := repo.ByEmail(ctx, email)
		if getErr != nil {
			return fmt.Errorf("lookup %q: %w", su.email, getErr)
		}
		user = existing
	default:
		return fmt.Errorf("save %q: %w", su.email, err)
	}

	return seedMembership(ctx, members, clock, log, user.ID(), su)
}

// seedMembership gán user làm thành viên workspace mặc định (idempotent).
func seedMembership(
	ctx context.Context,
	members *userpg.MembershipRepository,
	clock *usersys.Clock,
	log *logger.Logger,
	userID domain.UserID,
	su seedUser,
) error {
	wid, err := domain.NewWorkspaceID(_defaultWorkspaceID)
	if err != nil {
		return fmt.Errorf("seed workspace id: %w", err)
	}
	m, err := domain.NewMembership(wid, userID, su.role, clock.Now())
	if err != nil {
		return fmt.Errorf("seed membership: %w", err)
	}
	switch err := members.Save(ctx, m); {
	case err == nil:
		log.Infof(ctx, "seeded membership", "email", su.email+"="+su.role.String())
	case errors.Is(err, domain.ErrMembershipExists):
		log.Infof(ctx, "membership exists, skip", "email", su.email)
	default:
		return fmt.Errorf("save membership %q: %w", su.email, err)
	}
	return nil
}
