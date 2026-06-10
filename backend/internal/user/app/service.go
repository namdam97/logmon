// Package app chứa use case (application service) của bounded context user.
// Chỉ import domain và ports — không biết HTTP, DB hay framework cụ thể.
package app

import (
	"context"
	"fmt"

	"github.com/namdam97/logmon/backend/internal/user/domain"
	"github.com/namdam97/logmon/backend/internal/user/ports"
)

// Service điều phối các use case của user qua các dependency được inject.
type Service struct {
	repo   ports.UserRepository
	hasher ports.PasswordHasher
	ids    ports.IDGenerator
	clock  ports.Clock
}

// NewService tạo Service với các dependency bắt buộc (accept interfaces).
func NewService(
	repo ports.UserRepository,
	hasher ports.PasswordHasher,
	ids ports.IDGenerator,
	clock ports.Clock,
) *Service {
	return &Service{repo: repo, hasher: hasher, ids: ids, clock: clock}
}

// RegisterInput là dữ liệu vào cho use case đăng ký user.
type RegisterInput struct {
	Email    string
	Password string
}

const minPasswordLength = 8

// Register tạo user mới: validate input, băm mật khẩu, persist. Trả về user đã
// tạo hoặc domain error (ValidationError / ErrEmailTaken).
func (s *Service) Register(ctx context.Context, in RegisterInput) (domain.User, error) {
	email, err := domain.NewEmail(in.Email)
	if err != nil {
		return domain.User{}, err
	}
	if len(in.Password) < minPasswordLength {
		return domain.User{}, &domain.ValidationError{
			Field:   "password",
			Message: "must be at least 8 characters",
		}
	}

	hash, err := s.hasher.Hash(in.Password)
	if err != nil {
		return domain.User{}, fmt.Errorf("hash password: %w", err)
	}

	id, err := domain.NewUserID(s.ids.NewID())
	if err != nil {
		return domain.User{}, fmt.Errorf("new user id: %w", err)
	}

	user, err := domain.NewUser(id, email, hash, s.clock.Now())
	if err != nil {
		return domain.User{}, err
	}

	if err := s.repo.Save(ctx, user); err != nil {
		return domain.User{}, fmt.Errorf("save user: %w", err)
	}
	return user, nil
}

// Get lấy user theo id. Trả về domain.ErrUserNotFound nếu không tồn tại.
func (s *Service) Get(ctx context.Context, rawID string) (domain.User, error) {
	id, err := domain.NewUserID(rawID)
	if err != nil {
		return domain.User{}, err
	}
	user, err := s.repo.ByID(ctx, id)
	if err != nil {
		return domain.User{}, fmt.Errorf("get user: %w", err)
	}
	return user, nil
}
