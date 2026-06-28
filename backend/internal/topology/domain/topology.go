// Package domain mô hình service dependency graph (topology) suy ra từ traces.
// Read-only — không có lệnh thay đổi trạng thái; chỉ value object + hàm thuần để
// dựng graph và phân loại sức khoẻ node theo tỉ lệ lỗi.
package domain

import (
	"sort"
	"time"
)

// Ngưỡng phân loại sức khoẻ theo error rate (tỉ lệ span lỗi / tổng span gọi).
const (
	// _degradedThreshold: error rate ≥ 1% → degraded.
	_degradedThreshold = 0.01
	// _unhealthyThreshold: error rate ≥ 5% → unhealthy.
	_unhealthyThreshold = 0.05
)

// HealthStatus là trạng thái sức khoẻ của một service node.
type HealthStatus int

// Các trạng thái sức khoẻ (iota+1 — 0 là zero-value không hợp lệ).
const (
	// HealthHealthy: error rate dưới ngưỡng degraded.
	HealthHealthy HealthStatus = iota + 1
	// HealthDegraded: error rate trong [1%, 5%).
	HealthDegraded
	// HealthUnhealthy: error rate ≥ 5%.
	HealthUnhealthy
)

// String trả tên trạng thái ở dạng snake-case cho JSON/UI.
func (h HealthStatus) String() string {
	switch h {
	case HealthHealthy:
		return "healthy"
	case HealthDegraded:
		return "degraded"
	case HealthUnhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}

// Health phân loại sức khoẻ từ error rate. callCount = 0 → coi là healthy (không
// có lưu lượng thì không có lỗi).
func Health(errorRate float64) HealthStatus {
	switch {
	case errorRate >= _unhealthyThreshold:
		return HealthUnhealthy
	case errorRate >= _degradedThreshold:
		return HealthDegraded
	default:
		return HealthHealthy
	}
}

// Edge là một cạnh phụ thuộc source → target, kèm số lần gọi và số lỗi trong cửa
// sổ thời gian quan sát.
type Edge struct {
	Source     string
	Target     string
	CallCount  int64
	ErrorCount int64
}

// ErrorRate trả tỉ lệ lỗi của cạnh; 0 khi không có lưu lượng.
func (e Edge) ErrorRate() float64 {
	if e.CallCount <= 0 {
		return 0
	}
	return float64(e.ErrorCount) / float64(e.CallCount)
}

// Node là một service trong graph cùng số liệu tổng hợp và sức khoẻ.
type Node struct {
	Service    string
	Status     HealthStatus
	CallCount  int64
	ErrorCount int64
}

// ErrorRate trả tỉ lệ lỗi tổng hợp của node.
func (n Node) ErrorRate() float64 {
	if n.CallCount <= 0 {
		return 0
	}
	return float64(n.ErrorCount) / float64(n.CallCount)
}

// Graph là read model topology: tập node + cạnh, kèm mốc thời điểm dựng.
type Graph struct {
	Nodes       []Node
	Edges       []Edge
	GeneratedAt time.Time
}

// BuildGraph dựng graph từ danh sách cạnh. Mỗi service xuất hiện ở source hoặc
// target đều thành node; số liệu node = tổng outbound (mọi cạnh đi ra). Sức khoẻ
// node suy từ error rate outbound. Node và cạnh được sort ổn định để output xác
// định (dễ test + cache nhất quán).
func BuildGraph(edges []Edge, generatedAt time.Time) Graph {
	type agg struct {
		calls  int64
		errors int64
		seen   bool
	}
	stats := make(map[string]*agg)
	ensure := func(svc string) *agg {
		a, ok := stats[svc]
		if !ok {
			a = &agg{}
			stats[svc] = a
		}
		return a
	}

	out := make([]Edge, 0, len(edges))
	for _, e := range edges {
		if e.Source == "" || e.Target == "" {
			continue
		}
		src := ensure(e.Source)
		src.calls += e.CallCount
		src.errors += e.ErrorCount
		src.seen = true
		ensure(e.Target).seen = true
		out = append(out, e)
	}

	nodes := make([]Node, 0, len(stats))
	for svc, a := range stats {
		if !a.seen {
			continue
		}
		n := Node{Service: svc, CallCount: a.calls, ErrorCount: a.errors}
		n.Status = Health(n.ErrorRate())
		nodes = append(nodes, n)
	}

	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Service < nodes[j].Service })
	sort.Slice(out, func(i, j int) bool {
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		return out[i].Target < out[j].Target
	})

	return Graph{Nodes: nodes, Edges: out, GeneratedAt: generatedAt}
}
