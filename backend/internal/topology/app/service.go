// Package app chứa use case topology BC: trả service dependency graph theo
// workspace với cache-aside (materialize từ traces khi cache miss).
package app

import (
	"context"
	"fmt"
	"time"

	"github.com/namdam97/logmon/backend/internal/topology/domain"
	"github.com/namdam97/logmon/backend/internal/topology/ports"
)

const (
	// _window là cửa sổ quan sát traces để dựng graph (1 giờ gần nhất).
	_window = time.Hour
	// _cacheTTL là thời hạn cache graph (doc_v2/07 §2.10 — Redis TTL 5m).
	_cacheTTL = 5 * time.Minute
)

// Service trả topology graph với cache-aside.
type Service struct {
	reader ports.DependencyReader
	cache  ports.GraphCache
	clock  ports.Clock
}

// NewService tạo service. reader nil → graph rỗng (chưa cấu hình nguồn traces).
// cache nil → luôn dựng mới (không cache).
func NewService(reader ports.DependencyReader, cache ports.GraphCache, clock ports.Clock) *Service {
	return &Service{reader: reader, cache: cache, clock: clock}
}

// GetTopology trả graph cho workspace. Đọc cache trước; miss → dựng từ traces
// (cửa sổ 1h) rồi ghi cache TTL 5m. Lỗi cache (get/set) không chặn — degrade về
// dựng mới và log-less bỏ qua lỗi set.
func (s *Service) GetTopology(ctx context.Context, workspaceID string) (domain.Graph, error) {
	if s.cache != nil {
		if g, ok, err := s.cache.Get(ctx, workspaceID); err == nil && ok {
			return g, nil
		}
	}

	now := s.clock.Now()
	if s.reader == nil {
		return domain.BuildGraph(nil, now), nil
	}

	edges, err := s.reader.Dependencies(ctx, workspaceID, now.Add(-_window))
	if err != nil {
		return domain.Graph{}, fmt.Errorf("read dependencies: %w", err)
	}
	g := domain.BuildGraph(edges, now)

	if s.cache != nil {
		_ = s.cache.Set(ctx, workspaceID, g, _cacheTTL)
	}
	return g, nil
}
