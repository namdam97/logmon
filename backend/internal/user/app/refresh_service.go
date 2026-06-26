package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/namdam97/logmon/backend/internal/user/domain"
	"github.com/namdam97/logmon/backend/internal/user/ports"
)

// RefreshService quản lý vòng đời refresh token: phát hành, rotate (mỗi lần dùng
// sinh token mới + vô hiệu token cũ) và thu hồi. Phát hiện reuse: token đã rotate
// bị dùng lại → thu hồi toàn bộ family (ADR-023).
type RefreshService struct {
	repo   ports.RefreshTokenRepository
	codec  ports.RefreshTokenCodec
	tokens ports.TokenIssuer
	ids    ports.IDGenerator
	clock  ports.Clock
	ttl    time.Duration
	log    ports.Logger
}

// NewRefreshService tạo service với dependency được inject. log nil → nopLogger.
func NewRefreshService(
	repo ports.RefreshTokenRepository,
	codec ports.RefreshTokenCodec,
	tokens ports.TokenIssuer,
	ids ports.IDGenerator,
	clock ports.Clock,
	ttl time.Duration,
	log ports.Logger,
) *RefreshService {
	if log == nil {
		log = nopLogger{}
	}
	return &RefreshService{
		repo: repo, codec: codec, tokens: tokens, ids: ids, clock: clock, ttl: ttl, log: log,
	}
}

// Issue phát hành refresh token mới khởi tạo một family mới (lúc login). Trả về
// token thô để set vào cookie; DB chỉ lưu hash.
func (s *RefreshService) Issue(ctx context.Context, userID string) (string, error) {
	raw, err := s.mint(ctx, userID, s.ids.NewID())
	if err != nil {
		return "", fmt.Errorf("issue refresh token: %w", err)
	}
	return raw, nil
}

// TokenPair là cặp token trả về sau khi rotate.
type TokenPair struct {
	Access  string
	Refresh string
}

// Rotate đổi một refresh token lấy cặp access+refresh mới. Token cũ bị đánh dấu
// đã dùng (nguyên tử). Nếu token đã dùng bị trình lại → reuse → thu hồi family.
func (s *RefreshService) Rotate(ctx context.Context, rawRefresh string) (TokenPair, error) {
	hash := s.codec.Hash(rawRefresh)
	now := s.clock.Now()

	claimed, ok, err := s.repo.ClaimByHash(ctx, hash, now)
	if err != nil {
		return TokenPair{}, fmt.Errorf("claim refresh token: %w", err)
	}
	if !ok {
		return TokenPair{}, s.handleClaimMiss(ctx, hash)
	}

	access, err := s.tokens.Issue(claimed.UserID())
	if err != nil {
		return TokenPair{}, fmt.Errorf("issue access token: %w", err)
	}
	newRefresh, err := s.mint(ctx, claimed.UserID(), claimed.FamilyID())
	if err != nil {
		return TokenPair{}, fmt.Errorf("rotate refresh token: %w", err)
	}
	return TokenPair{Access: access, Refresh: newRefresh}, nil
}

// Revoke thu hồi family của một refresh token (logout). Idempotent: token không
// tồn tại → không lỗi.
func (s *RefreshService) Revoke(ctx context.Context, rawRefresh string) error {
	t, err := s.repo.ByHash(ctx, s.codec.Hash(rawRefresh))
	if err != nil {
		if errors.Is(err, domain.ErrRefreshTokenInvalid) {
			return nil
		}
		return fmt.Errorf("lookup refresh token: %w", err)
	}
	if err := s.repo.RevokeFamily(ctx, t.FamilyID()); err != nil {
		return fmt.Errorf("revoke refresh family: %w", err)
	}
	return nil
}

// handleClaimMiss phân biệt reuse (token đã rotate bị dùng lại) với token không
// tồn tại/hết hạn. Reuse → thu hồi family + ErrRefreshTokenReused.
func (s *RefreshService) handleClaimMiss(ctx context.Context, hash string) error {
	existing, err := s.repo.ByHash(ctx, hash)
	if err != nil {
		return domain.ErrRefreshTokenInvalid // không tồn tại
	}
	if existing.IsUsed() {
		if err := s.repo.RevokeFamily(ctx, existing.FamilyID()); err != nil {
			s.log.Error(ctx, err, "revoke family on refresh reuse")
		}
		return domain.ErrRefreshTokenReused
	}
	return domain.ErrRefreshTokenInvalid // tồn tại nhưng hết hạn
}

// mint sinh token thô, băm và lưu vào family cho trước; trả về token thô.
func (s *RefreshService) mint(ctx context.Context, userID, familyID string) (string, error) {
	raw, err := s.codec.Generate()
	if err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	now := s.clock.Now()
	rt, err := domain.NewRefreshToken(domain.NewRefreshTokenInput{
		ID:        s.ids.NewID(),
		UserID:    userID,
		FamilyID:  familyID,
		TokenHash: s.codec.Hash(raw),
		ExpiresAt: now.Add(s.ttl),
		CreatedAt: now,
	})
	if err != nil {
		return "", fmt.Errorf("build token: %w", err)
	}
	if err := s.repo.Insert(ctx, rt); err != nil {
		return "", fmt.Errorf("persist token: %w", err)
	}
	return raw, nil
}
