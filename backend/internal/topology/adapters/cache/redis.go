package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/namdam97/logmon/backend/internal/topology/domain"
	"github.com/namdam97/logmon/backend/internal/topology/ports"
)

// _keyPrefix tiền tố key Redis cho graph đã materialize per workspace.
const _keyPrefix = "logmon:topology:"

// Redis lưu graph dạng JSON trong Redis với TTL. Multi-instance an toàn (mọi
// instance đọc chung graph đã materialize).
type Redis struct {
	rdb redis.Cmdable
}

var _ ports.GraphCache = (*Redis)(nil)

// NewRedis tạo cache backed bởi go-redis.
func NewRedis(rdb redis.Cmdable) *Redis {
	return &Redis{rdb: rdb}
}

func key(workspaceID string) string { return _keyPrefix + workspaceID }

// Get đọc + giải mã graph; miss (redis.Nil) → ok=false, không phải lỗi.
func (r *Redis) Get(ctx context.Context, workspaceID string) (domain.Graph, bool, error) {
	raw, err := r.rdb.Get(ctx, key(workspaceID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return domain.Graph{}, false, nil
	}
	if err != nil {
		return domain.Graph{}, false, fmt.Errorf("redis get: %w", err)
	}
	g, err := decodeGraph(raw)
	if err != nil {
		return domain.Graph{}, false, err
	}
	return g, true, nil
}

// Set mã hoá + ghi graph với TTL.
func (r *Redis) Set(ctx context.Context, workspaceID string, g domain.Graph, ttl time.Duration) error {
	raw, err := encodeGraph(g)
	if err != nil {
		return err
	}
	if err := r.rdb.Set(ctx, key(workspaceID), raw, ttl).Err(); err != nil {
		return fmt.Errorf("redis set: %w", err)
	}
	return nil
}

// wireGraph là dạng tuần tự hoá ổn định của domain.Graph (domain không gắn JSON
// tag để giữ thuần stdlib — adapter chịu trách nhiệm mapping).
type wireGraph struct {
	Nodes []struct {
		Service    string `json:"service"`
		Status     string `json:"status"`
		CallCount  int64  `json:"callCount"`
		ErrorCount int64  `json:"errorCount"`
	} `json:"nodes"`
	Edges []struct {
		Source     string `json:"source"`
		Target     string `json:"target"`
		CallCount  int64  `json:"callCount"`
		ErrorCount int64  `json:"errorCount"`
	} `json:"edges"`
	GeneratedAt time.Time `json:"generatedAt"`
}

func encodeGraph(g domain.Graph) ([]byte, error) {
	var w wireGraph
	w.GeneratedAt = g.GeneratedAt
	for _, n := range g.Nodes {
		w.Nodes = append(w.Nodes, struct {
			Service    string `json:"service"`
			Status     string `json:"status"`
			CallCount  int64  `json:"callCount"`
			ErrorCount int64  `json:"errorCount"`
		}{n.Service, n.Status.String(), n.CallCount, n.ErrorCount})
	}
	for _, e := range g.Edges {
		w.Edges = append(w.Edges, struct {
			Source     string `json:"source"`
			Target     string `json:"target"`
			CallCount  int64  `json:"callCount"`
			ErrorCount int64  `json:"errorCount"`
		}{e.Source, e.Target, e.CallCount, e.ErrorCount})
	}
	b, err := json.Marshal(w)
	if err != nil {
		return nil, fmt.Errorf("marshal graph: %w", err)
	}
	return b, nil
}

func decodeGraph(raw []byte) (domain.Graph, error) {
	var w wireGraph
	if err := json.Unmarshal(raw, &w); err != nil {
		return domain.Graph{}, fmt.Errorf("unmarshal graph: %w", err)
	}
	// Dựng lại từ cạnh để node.Status nhất quán với domain (single source of truth).
	edges := make([]domain.Edge, 0, len(w.Edges))
	for _, e := range w.Edges {
		edges = append(edges, domain.Edge{Source: e.Source, Target: e.Target, CallCount: e.CallCount, ErrorCount: e.ErrorCount})
	}
	return domain.BuildGraph(edges, w.GeneratedAt), nil
}
