// Package auth thuộc shared kernel: phát hành + xác thực JWT và middleware bảo
// vệ route. Dùng HS256; secret nạp từ env, KHÔNG hardcode.
package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const _issuer = "logmon"

// ErrInvalidToken báo token sai chữ ký, hết hạn hoặc sai định dạng.
var ErrInvalidToken = errors.New("invalid token")

// JWTService phát hành và phân tích access token HS256.
type JWTService struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

// NewJWTService tạo service với secret (bắt buộc không rỗng) và thời hạn token.
func NewJWTService(secret string, ttl time.Duration) (*JWTService, error) {
	if secret == "" {
		return nil, errors.New("jwt secret must not be empty")
	}
	if ttl <= 0 {
		return nil, errors.New("jwt ttl must be positive")
	}
	return &JWTService{secret: []byte(secret), ttl: ttl, now: time.Now}, nil
}

// Issue ký một token với subject = userID, kèm iat/exp/iss.
func (s *JWTService) Issue(userID string) (string, error) {
	now := s.now()
	claims := jwt.RegisteredClaims{
		Subject:   userID,
		Issuer:    _issuer,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(s.ttl)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signed, nil
}

// Parse xác thực token và trả về userID (subject). Trả về ErrInvalidToken nếu
// token không hợp lệ vì bất kỳ lý do gì (không lộ chi tiết ra ngoài).
func (s *JWTService) Parse(raw string) (string, error) {
	claims := &jwt.RegisteredClaims{}
	_, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return s.secret, nil
	}, jwt.WithIssuer(_issuer), jwt.WithValidMethods([]string{"HS256"}))
	if err != nil || claims.Subject == "" {
		return "", ErrInvalidToken
	}
	return claims.Subject, nil
}
