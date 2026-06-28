package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/topology/domain"
)

func TestHealth(t *testing.T) {
	tests := []struct {
		name string
		give float64
		want domain.HealthStatus
	}{
		{"zero is healthy", 0, domain.HealthHealthy},
		{"below degraded", 0.005, domain.HealthHealthy},
		{"at degraded threshold", 0.01, domain.HealthDegraded},
		{"mid degraded", 0.03, domain.HealthDegraded},
		{"at unhealthy threshold", 0.05, domain.HealthUnhealthy},
		{"above unhealthy", 0.5, domain.HealthUnhealthy},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, domain.Health(tt.give))
		})
	}
}

func TestEdgeErrorRate(t *testing.T) {
	require.Equal(t, 0.0, domain.Edge{CallCount: 0}.ErrorRate())
	require.InDelta(t, 0.1, domain.Edge{CallCount: 100, ErrorCount: 10}.ErrorRate(), 1e-9)
}

func TestBuildGraph(t *testing.T) {
	now := time.Unix(1_000, 0).UTC()
	edges := []domain.Edge{
		{Source: "gateway", Target: "users", CallCount: 100, ErrorCount: 0},
		{Source: "gateway", Target: "orders", CallCount: 100, ErrorCount: 10}, // 10% trên cạnh
		{Source: "orders", Target: "db", CallCount: 50, ErrorCount: 0},
		{Source: "", Target: "skip", CallCount: 5},  // bỏ: source rỗng
		{Source: "skip2", Target: "", CallCount: 5}, // bỏ: target rỗng
	}

	g := domain.BuildGraph(edges, now)

	require.Equal(t, now, g.GeneratedAt)
	// 3 cạnh hợp lệ (2 cạnh rỗng bị loại).
	require.Len(t, g.Edges, 3)
	// nodes: gateway, users, orders, db (4 service hợp lệ).
	require.Len(t, g.Nodes, 4)

	// sort ổn định: db, gateway, orders, users.
	require.Equal(t, "db", g.Nodes[0].Service)
	require.Equal(t, "gateway", g.Nodes[1].Service)

	// gateway: outbound 200 calls, 10 errors → 5% → unhealthy.
	gw := g.Nodes[1]
	require.Equal(t, int64(200), gw.CallCount)
	require.Equal(t, int64(10), gw.ErrorCount)
	require.Equal(t, domain.HealthUnhealthy, gw.Status)

	// users: chỉ là target, không outbound → 0 calls → healthy.
	users := g.Nodes[3]
	require.Equal(t, "users", users.Service)
	require.Equal(t, domain.HealthHealthy, users.Status)
}

func TestHealthStatusString(t *testing.T) {
	require.Equal(t, "healthy", domain.HealthHealthy.String())
	require.Equal(t, "degraded", domain.HealthDegraded.String())
	require.Equal(t, "unhealthy", domain.HealthUnhealthy.String())
	require.Equal(t, "unknown", domain.HealthStatus(0).String())
}
