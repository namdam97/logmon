package system

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"github.com/namdam97/logmon/backend/internal/user/ports"
)

// _refreshTokenBytes là entropy của refresh token thô (256 bit) — đủ chống đoán.
const _refreshTokenBytes = 32

// RefreshCodec sinh refresh token ngẫu nhiên (crypto/rand) và băm SHA-256 để lưu.
// Chỉ hash được lưu DB; token thô chỉ tồn tại trong cookie của client.
type RefreshCodec struct{}

var _ ports.RefreshTokenCodec = (*RefreshCodec)(nil)

// NewRefreshCodec tạo codec.
func NewRefreshCodec() *RefreshCodec { return &RefreshCodec{} }

// Generate trả về token thô base64url (an toàn cho cookie), 256-bit entropy.
func (RefreshCodec) Generate() (string, error) {
	b := make([]byte, _refreshTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Hash trả về SHA-256 hex của token thô. Hex giúp so khớp/lookup ổn định.
func (RefreshCodec) Hash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
