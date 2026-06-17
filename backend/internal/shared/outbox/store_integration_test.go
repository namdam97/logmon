//go:build integration

// Integration test cho outbox.Store — cần Postgres thật (DATABASE_URL) đã áp
// migrations. Chạy: make test-integration (hoặc go test -tags integration ./...).
package outbox_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/shared/outbox"
)

func newPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dburl := os.Getenv("DATABASE_URL")
	if dburl == "" {
		t.Skip("DATABASE_URL chưa set — bỏ qua integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dburl)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	_, err = pool.Exec(context.Background(), "TRUNCATE outbox_events RESTART IDENTITY")
	require.NoError(t, err)
	return pool
}

func saveEvent(t *testing.T, pool *pgxpool.Pool, store *outbox.Store, eventType string) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	err = store.Save(ctx, tx, outbox.Event{
		AggregateType: "Test",
		AggregateID:   uuid.NewString(),
		EventType:     eventType,
		Payload:       []byte(`{"k":1}`),
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
}

func countByStatus(t *testing.T, pool *pgxpool.Pool, status string) int {
	t.Helper()
	var n int
	err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM outbox_events WHERE status = $1", status).Scan(&n)
	require.NoError(t, err)
	return n
}

func TestStoreProcessBatch_PublishAndRetryToFailed(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	// maxRetries=2: event lỗi sẽ thành failed sau 2 lần xử lý.
	store := outbox.NewStore(pool, outbox.WithMaxRetries(2))

	saveEvent(t, pool, store, "ok")
	saveEvent(t, pool, store, "ok")
	saveEvent(t, pool, store, "fail")

	boom := errors.New("handler boom")
	dispatch := func(_ context.Context, e outbox.Event) error {
		if e.EventType == "fail" {
			return boom
		}
		return nil
	}

	// Lần 1: 2 ok → published, 1 fail → retry (vẫn pending).
	processed, failed, err := store.ProcessBatch(ctx, 100, dispatch)
	require.NoError(t, err)
	require.Equal(t, 3, processed)
	require.Equal(t, 0, failed)
	require.Equal(t, 2, countByStatus(t, pool, "published"))
	require.Equal(t, 1, countByStatus(t, pool, "pending"))

	// Lần 2: fail event retry_count chạm max → failed.
	processed, failed, err = store.ProcessBatch(ctx, 100, dispatch)
	require.NoError(t, err)
	require.Equal(t, 1, processed)
	require.Equal(t, 1, failed)
	require.Equal(t, 1, countByStatus(t, pool, "failed"))
	require.Equal(t, 0, countByStatus(t, pool, "pending"))

	// Lần 3: hết pending.
	processed, _, err = store.ProcessBatch(ctx, 100, dispatch)
	require.NoError(t, err)
	require.Equal(t, 0, processed)
}

func TestStoreOldestPendingAge(t *testing.T) {
	pool := newPool(t)
	store := outbox.NewStore(pool)

	_, ok, err := store.OldestPendingAge(context.Background())
	require.NoError(t, err)
	require.False(t, ok, "rỗng → ok=false")

	saveEvent(t, pool, store, "ok")
	age, ok, err := store.OldestPendingAge(context.Background())
	require.NoError(t, err)
	require.True(t, ok)
	require.GreaterOrEqual(t, age, time.Duration(0))
}
