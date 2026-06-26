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
	// UpdatePasswordHash cập nhật hash mật khẩu của user (lazy migration sang
	// thuật toán mới sau khi login thành công).
	UpdatePasswordHash(ctx context.Context, id domain.UserID, hash string) error
}

// PasswordHasher trừu tượng hoá thuật toán băm + đối chiếu mật khẩu (argon2id,
// verify được cả bcrypt cũ để lazy migration). App không biết thuật toán cụ thể.
type PasswordHasher interface {
	Hash(plain string) (string, error)
	// Verify trả về nil nếu plain khớp hash, ngược lại trả về lỗi.
	Verify(hash, plain string) error
	// NeedsRehash báo hash hiện tại có cần băm lại bằng thuật toán/tham số hiện
	// hành không (vd: hash bcrypt cũ → cần nâng cấp argon2id).
	NeedsRehash(hash string) bool
}

// Logger ghi log có cấu trúc — inject để app báo lỗi không nghiêm trọng mà không
// nuốt im lặng (vd: lazy migration thất bại nhưng login vẫn thành công).
type Logger interface {
	Error(ctx context.Context, err error, msg string)
}

// TokenIssuer phát hành access token (JWT) cho một user đã xác thực.
type TokenIssuer interface {
	// Issue trả về token đã ký cho userID.
	Issue(userID string) (string, error)
}

// RefreshTokenRepository lưu trữ refresh token (chỉ hash). Rotation cần một thao
// tác claim nguyên tử để an toàn với truy cập đồng thời.
type RefreshTokenRepository interface {
	// Insert lưu một refresh token mới (chưa dùng).
	Insert(ctx context.Context, t domain.RefreshToken) error
	// ClaimByHash đánh dấu used_at cho token chưa dùng & chưa hết hạn theo hash,
	// trả về token đã claim (ok=true). Nếu không có token hợp lệ để claim → ok=false.
	ClaimByHash(ctx context.Context, hash string, now time.Time) (claimed domain.RefreshToken, ok bool, err error)
	// ByHash lấy token theo hash bất kể trạng thái — dùng để phân biệt reuse với
	// token không tồn tại sau khi claim thất bại. ErrRefreshTokenInvalid nếu không có.
	ByHash(ctx context.Context, hash string) (domain.RefreshToken, error)
	// RevokeFamily xoá toàn bộ token thuộc một family (reuse detection / logout).
	RevokeFamily(ctx context.Context, familyID string) error
}

// RefreshTokenCodec sinh token thô (crypto/rand) và băm (SHA-256) để lưu/đối chiếu.
type RefreshTokenCodec interface {
	// Generate trả về token thô ngẫu nhiên, an toàn cho URL/cookie.
	Generate() (raw string, err error)
	// Hash trả về SHA-256 hex của token thô (đối chiếu constant-time qua so khớp chuỗi hex).
	Hash(raw string) string
}

// IDGenerator sinh định danh duy nhất cho user mới.
type IDGenerator interface {
	NewID() string
}

// Clock cung cấp thời gian hiện tại — inject để test xác định.
type Clock interface {
	Now() time.Time
}
