// chaos.go — Chaos injection: giả lỗi + latency bổ sung để test khả năng
// quan sát của platform. Thiết kế injectable để unit test deterministic.
package main

import (
	"time"

	"math/rand/v2" //nolint:gosec // math/rand/v2 là CSPRNG-seeded, đủ cho chaos injection non-crypto
)

// chaos điều khiển việc inject lỗi và latency vào request.
// randFn được inject để test có thể kiểm soát kết quả.
type chaos struct {
	errorRate      float64
	extraLatencyMS int
	randFn         func() float64
}

// newChaos tạo chaos mới. Khi randFn == nil → dùng rand.Float64 từ math/rand/v2.
func newChaos(errorRate float64, extraLatencyMS int, randFn func() float64) *chaos {
	if randFn == nil {
		randFn = rand.Float64 //nolint:gosec
	}
	return &chaos{
		errorRate:      errorRate,
		extraLatencyMS: extraLatencyMS,
		randFn:         randFn,
	}
}

// shouldError trả true nếu request này nên bị giả lỗi 500.
// Dùng strict less-than để errorRate=1.0 không bao giờ có false positive
// khi randFn trả đúng 1.0.
func (c *chaos) shouldError() bool {
	return c.randFn() < c.errorRate
}

// extraDelay trả duration latency ngẫu nhiên trong khoảng [0, extraLatencyMS) ms.
// Trả 0 khi extraLatencyMS == 0.
func (c *chaos) extraDelay() time.Duration {
	if c.extraLatencyMS == 0 {
		return 0
	}
	return time.Duration(c.randFn()*float64(c.extraLatencyMS)) * time.Millisecond
}
