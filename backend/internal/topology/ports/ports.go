// Package ports khai báo interface topology BC phụ thuộc (DIP). Implementation ở
// adapters (Elasticsearch reader đọc traces, cache Redis/in-memory).
package ports

import (
	"context"
	"time"

	"github.com/namdam97/logmon/backend/internal/topology/domain"
)

// DependencyReader đọc các cạnh phụ thuộc service→service từ traces trong cửa sổ
// [since, now]. Đã lọc theo workspace (isolation đa tenant). Trả slice rỗng + nil
// khi không có dữ liệu (degrade an toàn — topology là thông tin, không chặn nghiệp
// vụ).
type DependencyReader interface {
	Dependencies(ctx context.Context, workspaceID string, since time.Time) ([]domain.Edge, error)
}

// GraphCache lưu graph đã materialize per workspace để đọc nhanh (TTL ngắn).
type GraphCache interface {
	// Get trả graph cache; ok=false khi miss hoặc hết hạn.
	Get(ctx context.Context, workspaceID string) (domain.Graph, bool, error)
	// Set ghi graph với thời hạn ttl.
	Set(ctx context.Context, workspaceID string, g domain.Graph, ttl time.Duration) error
}

// Clock cung cấp thời gian hiện tại — inject để test xác định.
type Clock interface {
	Now() time.Time
}
