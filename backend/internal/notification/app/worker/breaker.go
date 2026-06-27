// Package worker tiêu thụ delivery queue và gửi message qua Sender, với retry
// backoff, circuit breaker per-channel và ghi history (GĐ3.2, hội đồng GĐ3).
package worker

import (
	"sync"
	"time"
)

// breakerState là trạng thái mạch của một channel.
type breakerState int

const (
	_closed   breakerState = iota // bình thường, cho qua
	_open                         // đang chặn, chờ cooldown
	_halfOpen                     // cho 1 probe để dò hồi phục
)

type channelBreaker struct {
	state    breakerState
	failures int
	openedAt time.Time
}

// Breaker là circuit breaker per-channel: sau threshold lần fail liên tiếp →
// open trong cooldown; hết cooldown → half-open (cho 1 probe). Thread-safe.
type Breaker struct {
	mu        sync.Mutex
	channels  map[string]*channelBreaker
	threshold int
	cooldown  time.Duration
	now       func() time.Time
}

// NewBreaker tạo breaker. threshold lần fail liên tiếp mở mạch; cooldown là thời
// gian chờ trước khi cho probe. now inject để test (nil → time.Now).
func NewBreaker(threshold int, cooldown time.Duration, now func() time.Time) *Breaker {
	if now == nil {
		now = time.Now
	}
	return &Breaker{
		channels:  make(map[string]*channelBreaker),
		threshold: threshold,
		cooldown:  cooldown,
		now:       now,
	}
}

func (b *Breaker) get(key string) *channelBreaker {
	cb, ok := b.channels[key]
	if !ok {
		cb = &channelBreaker{state: _closed}
		b.channels[key] = cb
	}
	return cb
}

// Allow cho biết có nên thử gửi qua channel này không. Open + đã qua cooldown →
// chuyển half-open và cho 1 probe.
func (b *Breaker) Allow(key string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	cb := b.get(key)
	switch cb.state {
	case _open:
		if b.now().Sub(cb.openedAt) >= b.cooldown {
			cb.state = _halfOpen
			return true
		}
		return false
	default:
		return true
	}
}

// Success reset mạch về closed.
func (b *Breaker) Success(key string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	cb := b.get(key)
	cb.state = _closed
	cb.failures = 0
}

// Failure tăng đếm; đạt threshold (hoặc fail khi half-open) → open.
func (b *Breaker) Failure(key string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	cb := b.get(key)
	if cb.state == _halfOpen {
		cb.state = _open
		cb.openedAt = b.now()
		return
	}
	cb.failures++
	if cb.failures >= b.threshold {
		cb.state = _open
		cb.openedAt = b.now()
	}
}
