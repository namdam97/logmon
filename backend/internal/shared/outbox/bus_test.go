package outbox_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/shared/outbox"
)

func TestBusDispatch(t *testing.T) {
	ctx := context.Background()

	t.Run("gọi mọi handler của đúng event type", func(t *testing.T) {
		bus := outbox.NewBus()
		var calls []string
		bus.Subscribe("AlertRuleCreated", func(_ context.Context, e outbox.Event) error {
			calls = append(calls, "h1:"+e.EventType)
			return nil
		})
		bus.Subscribe("AlertRuleCreated", func(_ context.Context, _ outbox.Event) error {
			calls = append(calls, "h2")
			return nil
		})
		bus.Subscribe("Other", func(_ context.Context, _ outbox.Event) error {
			calls = append(calls, "other")
			return nil
		})

		err := bus.Dispatch(ctx, outbox.Event{EventType: "AlertRuleCreated"})
		require.NoError(t, err)
		require.Equal(t, []string{"h1:AlertRuleCreated", "h2"}, calls)
	})

	t.Run("không handler → no-op nil", func(t *testing.T) {
		bus := outbox.NewBus()
		require.NoError(t, bus.Dispatch(ctx, outbox.Event{EventType: "Unknown"}))
	})

	t.Run("handler lỗi → trả lỗi, dừng tại handler lỗi", func(t *testing.T) {
		bus := outbox.NewBus()
		boom := errors.New("boom")
		called := 0
		bus.Subscribe("E", func(_ context.Context, _ outbox.Event) error { called++; return boom })
		bus.Subscribe("E", func(_ context.Context, _ outbox.Event) error { called++; return nil })

		err := bus.Dispatch(ctx, outbox.Event{EventType: "E"})
		require.ErrorIs(t, err, boom)
		require.Equal(t, 1, called, "handler sau không được gọi khi handler trước lỗi")
	})
}
