// Package cache cài đặt ports.GraphCache. Memory dùng cho single-instance/dev;
// Redis cho multi-instance prod (chung TTL, key theo workspace).
package cache

import (
	"context"
	"sync"
	"time"

	"github.com/namdam97/logmon/backend/internal/topology/domain"
	"github.com/namdam97/logmon/backend/internal/topology/ports"
)

// Memory là cache in-memory có TTL, an toàn goroutine. Phù hợp single-instance;
// multi-instance prod nên dùng Redis để chia sẻ graph đã materialize.
type Memory struct {
	mu    sync.Mutex
	items map[string]memoryItem
	now   func() time.Time
}

type memoryItem struct {
	graph     domain.Graph
	expiresAt time.Time
}

var _ ports.GraphCache = (*Memory)(nil)

// NewMemory tạo cache rỗng. now nil → time.Now (inject để test hết hạn xác định).
func NewMemory(now func() time.Time) *Memory {
	if now == nil {
		now = time.Now
	}
	return &Memory{items: make(map[string]memoryItem), now: now}
}

// Get trả graph nếu còn hạn; hết hạn → xoá + miss.
func (m *Memory) Get(_ context.Context, workspaceID string) (domain.Graph, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	it, ok := m.items[workspaceID]
	if !ok {
		return domain.Graph{}, false, nil
	}
	if !m.now().Before(it.expiresAt) {
		delete(m.items, workspaceID)
		return domain.Graph{}, false, nil
	}
	return it.graph, true, nil
}

// Set ghi graph với thời hạn ttl.
func (m *Memory) Set(_ context.Context, workspaceID string, g domain.Graph, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[workspaceID] = memoryItem{graph: g, expiresAt: m.now().Add(ttl)}
	return nil
}
