package main

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

// healthCheck phục vụ Docker HEALTHCHECK trong image distroless (KHÔNG có
// wget/curl/shell). GET /healthz trên loopback; exit 0 nếu 200, ngược lại exit 1.
// Gọi qua: `/app/userservice healthcheck`.
func healthCheck() {
	port := envOr("PORT", _defaultPort)
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		os.Exit(1)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck: status", resp.StatusCode)
		os.Exit(1)
	}
}
