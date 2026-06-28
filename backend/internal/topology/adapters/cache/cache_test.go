package cache

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/topology/domain"
)

func TestMemoryCacheSetGet(t *testing.T) {
	clk := &mockClock{t: time.Unix(0, 0).UTC()}
	c := NewMemory(clk.now)
	g := domain.BuildGraph([]domain.Edge{{Source: "a", Target: "b", CallCount: 1}}, time.Unix(1, 0).UTC())

	require.NoError(t, c.Set(context.Background(), "ws-1", g, time.Minute))

	got, ok, err := c.Get(context.Background(), "ws-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Len(t, got.Nodes, 2)
}

func TestMemoryCacheMiss(t *testing.T) {
	c := NewMemory(func() time.Time { return time.Unix(0, 0).UTC() })
	_, ok, err := c.Get(context.Background(), "absent")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestMemoryCacheExpiry(t *testing.T) {
	clk := &mockClock{t: time.Unix(0, 0).UTC()}
	c := NewMemory(clk.now)
	require.NoError(t, c.Set(context.Background(), "ws-1", domain.Graph{}, time.Minute))

	clk.t = time.Unix(61, 0).UTC() // qua TTL 60s
	_, ok, err := c.Get(context.Background(), "ws-1")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	g := domain.BuildGraph([]domain.Edge{
		{Source: "gateway", Target: "orders", CallCount: 100, ErrorCount: 10},
	}, time.Unix(1_000, 0).UTC())

	raw, err := encodeGraph(g)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"unhealthy"`) // gateway 10% → unhealthy

	got, err := decodeGraph(raw)
	require.NoError(t, err)
	require.Equal(t, g.GeneratedAt, got.GeneratedAt)
	require.Len(t, got.Edges, 1)
	require.Equal(t, "gateway", got.Nodes[0].Service)
	require.Equal(t, domain.HealthUnhealthy, got.Nodes[0].Status)
}

type mockClock struct{ t time.Time }

func (m *mockClock) now() time.Time { return m.t }
