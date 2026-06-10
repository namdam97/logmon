// Package ports khai báo interfaces mà tầng app phụ thuộc (DIP). Implementation
// nằm ở adapters. Interfaces giữ nhỏ và tập trung (ISP).
package ports

import (
	"context"
	"time"

	"github.com/namdam97/logmon/backend/internal/user/domain"
)

// UserRepository trừu tượng hoá lưu trữ user.
type UserRepository interface {
	// Save lưu user mới. Trả về domain.ErrEmailTaken nếu email đã tồn tại.
	Save(ctx context.Context, u domain.User) error
	// ByID lấy user theo id. Trả về domain.ErrUserNotFound nếu không có.
	ByID(ctx context.Context, id domain.UserID) (domain.User, error)
	// ByEmail lấy user theo email. Trả về domain.ErrUserNotFound nếu không có.
	ByEmail(ctx context.Context, email domain.Email) (domain.User, error)
}

// PasswordHasher trừu tượng hoá thuật toán băm + đối chiếu mật khẩu (bcrypt).
type PasswordHasher interface {
	Hash(plain string) (string, error)
	// Verify trả về nil nếu plain khớp hash, ngược lại trả về lỗi.
	Verify(hash, plain string) error
}

// TokenIssuer phát hành access token (JWT) cho một user đã xác thực.
type TokenIssuer interface {
	// Issue trả về token đã ký cho userID.
	Issue(userID string) (string, error)
}

// IDGenerator sinh định danh duy nhất cho user mới.
type IDGenerator interface {
	NewID() string
}

// Clock cung cấp thời gian hiện tại — inject để test xác định.
type Clock interface {
	Now() time.Time
}
