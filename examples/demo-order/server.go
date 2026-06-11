// server.go — Router Gin + handlers cho demo-order service.
package main

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// order đại diện cho một đơn hàng trong bộ nhớ.
type order struct {
	ID       string `json:"id"`
	Item     string `json:"item"`
	Quantity int    `json:"quantity"`
}

// createOrderRequest là body của POST /api/v1/orders.
type createOrderRequest struct {
	Item     string `json:"item"     binding:"required"`
	Quantity int    `json:"quantity" binding:"required,gt=0"`
}

// appServer gom router, logger, metrics, chaos và dữ liệu in-memory.
type appServer struct {
	log   *serviceLogger
	mx    *appMetrics
	chaos *chaos

	mu     sync.RWMutex // bảo vệ orders — Gin xử lý request trên nhiều goroutine
	orders []order
}

// buildServer khởi tạo Gin engine với tất cả middleware và handler.
func buildServer(log *serviceLogger, mx *appMetrics, ch *chaos) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	s := &appServer{
		log:   log,
		mx:    mx,
		chaos: ch,
		orders: []order{
			{ID: "order-001", Item: "widget-A", Quantity: 10},
			{ID: "order-002", Item: "widget-B", Quantity: 5},
			{ID: "order-003", Item: "gadget-X", Quantity: 2},
		},
	}

	// /healthz và /metrics không qua chaos, không log request thông thường.
	r.GET("/healthz", s.handleHealthz)
	r.GET("/metrics", gin.WrapH(promhttp.HandlerFor(mx.Registry(), promhttp.HandlerOpts{})))

	// Nhóm /api/v1 có đầy đủ middleware: in-flight, chaos, observe, log.
	api := r.Group("/api/v1")
	api.Use(s.middlewareInFlight())
	api.Use(s.middlewareChaos())
	api.Use(s.middlewareObserve())

	api.GET("/orders", s.handleListOrders)
	api.POST("/orders", s.handleCreateOrder)

	return r
}

// handleHealthz trả {"status":"ok"} — không bị chaos injection.
func (s *appServer) handleHealthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// handleListOrders trả danh sách orders in-memory. Trả về bản copy để caller
// không giữ reference vào slice nội bộ (copy tại API boundary).
func (s *appServer) handleListOrders(c *gin.Context) {
	s.mu.RLock()
	result := make([]order, len(s.orders))
	copy(result, s.orders)
	s.mu.RUnlock()
	c.JSON(http.StatusOK, gin.H{"orders": result})
}

// handleCreateOrder validate body rồi tạo order mới.
func (s *appServer) handleCreateOrder(c *gin.Context) {
	var req createOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	newOrder := order{
		ID:       fmt.Sprintf("order-%d", time.Now().UnixNano()),
		Item:     req.Item,
		Quantity: req.Quantity,
	}
	s.mu.Lock()
	s.orders = append(s.orders, newOrder)
	s.mu.Unlock()
	c.JSON(http.StatusCreated, newOrder)
}

// middlewareInFlight tăng/giảm gauge số request đang xử lý.
func (s *appServer) middlewareInFlight() gin.HandlerFunc {
	return func(c *gin.Context) {
		s.mx.InFlightInc()
		defer s.mx.InFlightDec()
		c.Next()
	}
}

// middlewareChaos inject lỗi 500 và/hoặc latency theo config.
func (s *appServer) middlewareChaos() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Áp dụng latency bổ sung trước.
		if d := s.chaos.extraDelay(); d > 0 {
			time.Sleep(d)
		}
		// Nếu bị inject lỗi → dừng chuỗi handler.
		if s.chaos.shouldError() {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "simulated error"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// middlewareObserve ghi metrics và log sau khi handler xong.
func (s *appServer) middlewareObserve() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		dur := time.Since(start)

		status := c.Writer.Status()
		path := c.FullPath()
		method := c.Request.Method

		s.mx.ObserveRequest(method, path, status, dur)
		s.log.Request(method, path, status, int(dur.Milliseconds()))
	}
}
