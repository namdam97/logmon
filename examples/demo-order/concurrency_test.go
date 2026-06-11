package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// TestOrdersConcurrentAccess là regression test cho data race trên s.orders:
// GET và POST chạy song song trên nhiều goroutine — race detector (-race)
// sẽ fail nếu thiếu mutex bảo vệ slice.
func TestOrdersConcurrentAccess(t *testing.T) {
	srv := buildServer(newLogger(io.Discard, "error"), newMetrics(), newChaos(0, 0, nil))

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil)
			srv.ServeHTTP(httptest.NewRecorder(), req)
		}()
		go func() {
			defer wg.Done()
			body := bytes.NewBufferString(`{"item":"widget","quantity":1}`)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", body)
			req.Header.Set("Content-Type", "application/json")
			srv.ServeHTTP(httptest.NewRecorder(), req)
		}()
	}
	wg.Wait()
}
