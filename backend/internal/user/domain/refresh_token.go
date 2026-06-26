package domain

import "time"

// RefreshToken là một refresh token đã phát hành (chỉ giữ hash, không giữ token
// thô). Bất biến: accessor trả về giá trị, không cho sửa trực tiếp. family_id gom
// các token cùng chuỗi rotation để phát hiện reuse.
type RefreshToken struct {
	id        string
	userID    string
	familyID  string
	tokenHash string
	usedAt    *time.Time
	expiresAt time.Time
	createdAt time.Time
}

// NewRefreshTokenInput là dữ liệu tạo một refresh token mới (chưa dùng).
type NewRefreshTokenInput struct {
	ID        string
	UserID    string
	FamilyID  string
	TokenHash string
	ExpiresAt time.Time
	CreatedAt time.Time
}

// NewRefreshToken validate và tạo refresh token mới (usedAt = nil).
func NewRefreshToken(in NewRefreshTokenInput) (RefreshToken, error) {
	switch {
	case in.ID == "":
		return RefreshToken{}, newValidationError("id", "must not be empty")
	case in.UserID == "":
		return RefreshToken{}, newValidationError("userID", "must not be empty")
	case in.FamilyID == "":
		return RefreshToken{}, newValidationError("familyID", "must not be empty")
	case in.TokenHash == "":
		return RefreshToken{}, newValidationError("tokenHash", "must not be empty")
	case !in.ExpiresAt.After(in.CreatedAt):
		return RefreshToken{}, newValidationError("expiresAt", "must be after createdAt")
	}
	return RefreshToken{
		id:        in.ID,
		userID:    in.UserID,
		familyID:  in.FamilyID,
		tokenHash: in.TokenHash,
		expiresAt: in.ExpiresAt,
		createdAt: in.CreatedAt,
	}, nil
}

// ReconstructRefreshToken hydrate token từ DB (gồm usedAt nullable). Không
// validate — dữ liệu đã hợp lệ khi lưu.
func ReconstructRefreshToken(id, userID, familyID, tokenHash string, usedAt *time.Time, expiresAt, createdAt time.Time) RefreshToken {
	return RefreshToken{
		id:        id,
		userID:    userID,
		familyID:  familyID,
		tokenHash: tokenHash,
		usedAt:    usedAt,
		expiresAt: expiresAt,
		createdAt: createdAt,
	}
}

// ID trả về định danh token.
func (t RefreshToken) ID() string { return t.id }

// UserID trả về user sở hữu token.
func (t RefreshToken) UserID() string { return t.userID }

// FamilyID trả về định danh chuỗi rotation.
func (t RefreshToken) FamilyID() string { return t.familyID }

// TokenHash trả về SHA-256 hex của token thô.
func (t RefreshToken) TokenHash() string { return t.tokenHash }

// ExpiresAt trả về thời điểm hết hạn.
func (t RefreshToken) ExpiresAt() time.Time { return t.expiresAt }

// CreatedAt trả về thời điểm tạo.
func (t RefreshToken) CreatedAt() time.Time { return t.createdAt }

// IsUsed báo token đã được rotate (dùng) hay chưa.
func (t RefreshToken) IsUsed() bool { return t.usedAt != nil }

// IsExpired báo token đã hết hạn tại thời điểm now.
func (t RefreshToken) IsExpired(now time.Time) bool { return !now.Before(t.expiresAt) }
