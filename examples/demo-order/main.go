// Command demo-order là HTTP service mẫu instrument đầy đủ, sinh telemetry cho
// dev/test platform LogMon. main() chỉ gọi run() và exit một lần — toàn bộ
// logic khởi tạo + graceful shutdown nằm trong run().
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

const (
	_defaultPort       = "8081"
	_defaultLogLevel   = "info"
	_shutdownTimeout   = 10 * time.Second
	_readHeaderTimeout = 5 * time.Second
	_defaultErrorRate  = 0.02
	_defaultExtraLatMS = 0
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "demo-order:", err)
		os.Exit(1)
	}
}

// config chứa toàn bộ cấu hình đọc từ biến môi trường.
type config struct {
	port           string
	logLevel       string
	errorRate      float64 // tỉ lệ giả lỗi 500 cho /api/v1/orders
	extraLatencyMS int     // latency ngẫu nhiên bổ sung (ms)
}

// loadConfig đọc biến môi trường, fallback về giá trị mặc định hợp lý.
func loadConfig() config {
	errorRate := _defaultErrorRate
	if v := os.Getenv("ERROR_RATE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 && f <= 1 {
			errorRate = f
		}
	}

	extraLatMS := _defaultExtraLatMS
	if v := os.Getenv("EXTRA_LATENCY_MS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			extraLatMS = n
		}
	}

	return config{
		port:           envOr("PORT", _defaultPort),
		logLevel:       envOr("LOG_LEVEL", _defaultLogLevel),
		errorRate:      errorRate,
		extraLatencyMS: extraLatMS,
	}
}

func run() error {
	cfg := loadConfig()

	log := newLogger(os.Stdout, cfg.logLevel)

	mx := newMetrics()

	chaos := newChaos(cfg.errorRate, cfg.extraLatencyMS, nil)

	srv := buildServer(log, mx, chaos)

	httpSrv := &http.Server{
		Addr:              ":" + cfg.port,
		Handler:           srv,
		ReadHeaderTimeout: _readHeaderTimeout,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Info("demo-order service khởi động", "addr", httpSrv.Addr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
		log.Info("nhận shutdown signal, dừng gracefully")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), _shutdownTimeout)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	return nil
}

// envOr đọc biến môi trường; trả fallback nếu rỗng.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
