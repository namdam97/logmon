package worker

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBreakerOpensAfterThreshold(t *testing.T) {
	b := NewBreaker(3, time.Minute, func() time.Time { return time.Unix(0, 0) })

	require.True(t, b.Allow("ch"))
	b.Failure("ch")
	b.Failure("ch")
	require.True(t, b.Allow("ch"), "below threshold still allowed")
	b.Failure("ch")
	require.False(t, b.Allow("ch"), "open after threshold")
}

func TestBreakerSuccessResets(t *testing.T) {
	b := NewBreaker(2, time.Minute, func() time.Time { return time.Unix(0, 0) })
	b.Failure("ch")
	b.Success("ch")
	b.Failure("ch")
	require.True(t, b.Allow("ch"), "counter reset by success")
}

func TestBreakerHalfOpenAfterCooldown(t *testing.T) {
	now := time.Unix(0, 0)
	b := NewBreaker(1, 30*time.Second, func() time.Time { return now })
	b.Failure("ch")
	require.False(t, b.Allow("ch"))

	now = now.Add(31 * time.Second)
	require.True(t, b.Allow("ch"), "half-open probe after cooldown")

	// probe fails → reopen
	b.Failure("ch")
	require.False(t, b.Allow("ch"))
}

func TestBreakerHalfOpenSuccessCloses(t *testing.T) {
	now := time.Unix(0, 0)
	b := NewBreaker(1, 30*time.Second, func() time.Time { return now })
	b.Failure("ch")
	now = now.Add(31 * time.Second)
	require.True(t, b.Allow("ch"))
	b.Success("ch")
	require.True(t, b.Allow("ch"), "closed after successful probe")
}

func TestBreakerIsolatesChannels(t *testing.T) {
	b := NewBreaker(1, time.Minute, func() time.Time { return time.Unix(0, 0) })
	b.Failure("a")
	require.False(t, b.Allow("a"))
	require.True(t, b.Allow("b"), "other channel unaffected")
}
