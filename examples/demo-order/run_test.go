package main

import (
	"net/http"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestRunStartsAndShutsDown verify vòng đời đầy đủ của run(): khởi động, phục
// vụ /healthz, rồi dừng gracefully khi nhận SIGINT.
func TestRunStartsAndShutsDown(t *testing.T) {
	t.Setenv("PORT", "18187")
	t.Setenv("ERROR_RATE", "0")
	t.Setenv("EXTRA_LATENCY_MS", "0")
	t.Setenv("LOG_LEVEL", "error")

	done := make(chan error, 1)
	go func() { done <- run() }()

	// Chờ server sẵn sàng (tối đa 2.5s).
	var ready bool
	for range 50 {
		resp, err := http.Get("http://localhost:18187/healthz")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				ready = true
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.True(t, ready, "server không sẵn sàng trong 2.5s")

	// Gửi SIGINT cho chính process — signal.NotifyContext trong run() bắt được
	// và kích hoạt graceful shutdown.
	proc, err := os.FindProcess(os.Getpid())
	require.NoError(t, err)
	require.NoError(t, proc.Signal(syscall.SIGINT))

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("run() không dừng trong 5s sau SIGINT")
	}
}
