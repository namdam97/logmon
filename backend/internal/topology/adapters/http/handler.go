// Package http expose topology BC qua REST (doc_v2/07 §2.10 GET /topology).
// Tenant-scoped, read-only — chỉ cần membership (viewer+).
package http

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/namdam97/logmon/backend/internal/shared/auth"
	"github.com/namdam97/logmon/backend/internal/shared/httpx"
	"github.com/namdam97/logmon/backend/internal/topology/domain"
)

const _timeLayout = "2006-01-02T15:04:05Z07:00"

// topologyService là use case handler phụ thuộc (ISP).
type topologyService interface {
	GetTopology(ctx context.Context, workspaceID string) (domain.Graph, error)
}

// Handler gắn topology use case vào HTTP routes.
type Handler struct {
	svc topologyService
}

// NewHandler tạo handler.
func NewHandler(svc topologyService) *Handler {
	return &Handler{svc: svc}
}

// Register gắn route. authMW = tenantMW (đảm bảo viewer+). Read-only.
func (h *Handler) Register(rg *gin.RouterGroup, authMW gin.HandlerFunc) {
	rg.GET("/topology", authMW, h.topology)
}

type nodeResponse struct {
	Service    string  `json:"service"`
	Status     string  `json:"status"`
	CallCount  int64   `json:"callCount"`
	ErrorCount int64   `json:"errorCount"`
	ErrorRate  float64 `json:"errorRate"`
}

type edgeResponse struct {
	Source     string  `json:"source"`
	Target     string  `json:"target"`
	CallCount  int64   `json:"callCount"`
	ErrorCount int64   `json:"errorCount"`
	ErrorRate  float64 `json:"errorRate"`
}

type graphResponse struct {
	Nodes       []nodeResponse `json:"nodes"`
	Edges       []edgeResponse `json:"edges"`
	GeneratedAt string         `json:"generatedAt"`
}

func toGraph(g domain.Graph) graphResponse {
	nodes := make([]nodeResponse, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		nodes = append(nodes, nodeResponse{
			Service: n.Service, Status: n.Status.String(),
			CallCount: n.CallCount, ErrorCount: n.ErrorCount, ErrorRate: n.ErrorRate(),
		})
	}
	edges := make([]edgeResponse, 0, len(g.Edges))
	for _, e := range g.Edges {
		edges = append(edges, edgeResponse{
			Source: e.Source, Target: e.Target,
			CallCount: e.CallCount, ErrorCount: e.ErrorCount, ErrorRate: e.ErrorRate(),
		})
	}
	return graphResponse{Nodes: nodes, Edges: edges, GeneratedAt: g.GeneratedAt.Format(_timeLayout)}
}

func (h *Handler) topology(c *gin.Context) {
	g, err := h.svc.GetTopology(c.Request.Context(), h.wsID(c))
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "internal server error")
		return
	}
	httpx.OK(c, http.StatusOK, toGraph(g))
}

func (h *Handler) wsID(c *gin.Context) string {
	ws, _ := auth.WorkspaceIDFromContext(c)
	return ws
}
