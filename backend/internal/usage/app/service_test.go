package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/usage/app"
	"github.com/namdam97/logmon/backend/internal/usage/domain"
)

type fakeQuotaStore struct {
	q    *domain.Quota
	save int
}

func (s *fakeQuotaStore) Get(_ context.Context, _ string) (domain.Quota, error) {
	if s.q == nil {
		return domain.Quota{}, domain.ErrQuotaNotFound
	}
	return *s.q, nil
}
func (s *fakeQuotaStore) Upsert(_ context.Context, q domain.Quota) error {
	s.save++
	cp := q
	s.q = &cp
	return nil
}

type fakeUsageReader struct {
	ingestion, storage, logs int64
}

func (f fakeUsageReader) IngestionBytes(context.Context, string, time.Time) (int64, error) {
	return f.ingestion, nil
}
func (f fakeUsageReader) StorageBytes(context.Context, string) (int64, error) {
	return f.storage, nil
}
func (f fakeUsageReader) LogCount(context.Context, string, time.Time) (int64, error) {
	return f.logs, nil
}

type clk struct{ t time.Time }

func (c clk) Now() time.Time { return c.t }

func TestGetUsageComputesCost(t *testing.T) {
	const gb = 1 << 30
	svc := app.NewService(&fakeQuotaStore{}, fakeUsageReader{ingestion: 2 * gb, storage: 10 * gb, logs: 5000}, clk{t: time.Unix(1_000_000, 0).UTC()})
	u, err := svc.GetUsage(context.Background(), "ws-1")
	require.NoError(t, err)
	require.Equal(t, int64(2*gb), u.IngestionBytes)
	require.Equal(t, int64(5000), u.LogCount)
	// cost = 2*0.50 + 10*0.10 = 2.0
	require.InDelta(t, 2.0, u.EstimatedCostUSD, 0.001)
}

func TestGetUsageNilReader(t *testing.T) {
	svc := app.NewService(&fakeQuotaStore{}, nil, clk{t: time.Unix(1, 0).UTC()})
	u, err := svc.GetUsage(context.Background(), "ws-1")
	require.NoError(t, err)
	require.Equal(t, int64(0), u.IngestionBytes)
}

func TestGetQuotaDefaultsWhenMissing(t *testing.T) {
	svc := app.NewService(&fakeQuotaStore{}, nil, clk{t: time.Unix(1, 0).UTC()})
	q, err := svc.GetQuota(context.Background(), "ws-1")
	require.NoError(t, err)
	require.Equal(t, int64(10*(1<<30)), q.MaxIngestionBytesPerDay())
	require.Equal(t, 30, q.RetentionDays())
}

func TestSetQuotaValidatesAndPersists(t *testing.T) {
	store := &fakeQuotaStore{}
	svc := app.NewService(store, nil, clk{t: time.Unix(1, 0).UTC()})

	_, err := svc.SetQuota(context.Background(), app.SetQuotaInput{WorkspaceID: "ws-1", MaxIngestionBytesPerDay: 0, MaxStorageBytes: 1, RetentionDays: 7})
	require.Error(t, err)
	require.Equal(t, 0, store.save)

	q, err := svc.SetQuota(context.Background(), app.SetQuotaInput{WorkspaceID: "ws-1", MaxIngestionBytesPerDay: 5 << 30, MaxStorageBytes: 50 << 30, RetentionDays: 14})
	require.NoError(t, err)
	require.Equal(t, 14, q.RetentionDays())
	require.Equal(t, 1, store.save)
}

func TestQuotaExceeded(t *testing.T) {
	q := domain.DefaultQuota("ws-1", time.Unix(1, 0).UTC())
	require.True(t, q.IngestionExceeded(q.MaxIngestionBytesPerDay()+1))
	require.False(t, q.IngestionExceeded(q.MaxIngestionBytesPerDay()))
	require.True(t, q.StorageExceeded(q.MaxStorageBytes()+1))
}
