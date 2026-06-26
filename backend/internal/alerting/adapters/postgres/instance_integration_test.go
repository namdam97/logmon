//go:build integration

// Integration test cho InstanceRepository — cần Postgres thật (DATABASE_URL) đã
// áp migrations. Chạy: make test-integration.
package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/alerting/adapters/postgres"
	"github.com/namdam97/logmon/backend/internal/alerting/domain"
)

func firingInstance(t *testing.T, ws, fp string, firedAt time.Time) domain.AlertInstance {
	t.Helper()
	fingerprint, err := domain.NewFingerprint(fp)
	require.NoError(t, err)
	inst, err := domain.NewFiringInstance(domain.NewFiringInstanceInput{
		ID:          uuid.NewString(),
		WorkspaceID: ws,
		Fingerprint: fingerprint,
		FiredAt:     firedAt,
		Labels:      map[string]string{"alertname": "HighErrorRate", "severity": "critical"},
	})
	require.NoError(t, err)
	return inst
}

func TestInstanceRepository_UpsertIsIdempotent(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	repo := postgres.NewInstanceRepository(pool)
	ws := uuid.NewString()
	firedAt := time.Now().UTC().Truncate(time.Millisecond)

	require.NoError(t, repo.UpsertFiring(ctx, firingInstance(t, ws, "fp-1", firedAt)))
	// Webhook lặp cùng (fingerprint, fired_at) → không tạo bản trùng.
	require.NoError(t, repo.UpsertFiring(ctx, firingInstance(t, ws, "fp-1", firedAt)))

	require.Equal(t, 1, count(t, pool, "SELECT count(*) FROM alert_instances WHERE fingerprint = $1", "fp-1"))

	active, err := repo.ListActive(ctx, ws)
	require.NoError(t, err)
	require.Len(t, active, 1)
	require.Equal(t, domain.InstanceFiring, active[0].Status())
	require.Equal(t, "HighErrorRate", active[0].Labels()["alertname"])
}

func TestInstanceRepository_ResolveRemovesFromActive(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	repo := postgres.NewInstanceRepository(pool)
	ws := uuid.NewString()
	firedAt := time.Now().UTC().Truncate(time.Millisecond)

	require.NoError(t, repo.UpsertFiring(ctx, firingInstance(t, ws, "fp-2", firedAt)))
	require.NoError(t, repo.Resolve(ctx, ws, "fp-2", time.Now().UTC()))

	active, err := repo.ListActive(ctx, ws)
	require.NoError(t, err)
	require.Empty(t, active, "instance đã resolved không còn trong active")

	// Resolve lặp cho fingerprint đã resolved → no-op, không lỗi.
	require.NoError(t, repo.Resolve(ctx, ws, "fp-2", time.Now().UTC()))
}

func TestInstanceRepository_AcknowledgePersistsAndStaysActive(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	repo := postgres.NewInstanceRepository(pool)
	ws := uuid.NewString()
	actor := uuid.NewString()
	firedAt := time.Now().UTC().Truncate(time.Millisecond)
	ackedAt := firedAt.Add(30 * time.Minute)

	inst := firingInstance(t, ws, "fp-ack", firedAt)
	require.NoError(t, repo.UpsertFiring(ctx, inst))

	loaded, err := repo.ByID(ctx, ws, inst.ID())
	require.NoError(t, err)
	require.Equal(t, domain.InstanceFiring, loaded.Status())

	acked, err := loaded.Acknowledge(actor, ackedAt)
	require.NoError(t, err)
	require.NoError(t, repo.Acknowledge(ctx, acked))

	got, err := repo.ByID(ctx, ws, inst.ID())
	require.NoError(t, err)
	require.Equal(t, domain.InstanceAcknowledged, got.Status())
	require.Equal(t, actor, got.AcknowledgedBy())
	require.Equal(t, ackedAt, got.AcknowledgedAt())

	// acknowledged vẫn nằm trong active (chưa resolved).
	active, err := repo.ListActive(ctx, ws)
	require.NoError(t, err)
	require.Len(t, active, 1)
	require.Equal(t, domain.InstanceAcknowledged, active[0].Status())
}

func TestInstanceRepository_ByIDNotFound(t *testing.T) {
	pool := newPool(t)
	repo := postgres.NewInstanceRepository(pool)

	_, err := repo.ByID(context.Background(), uuid.NewString(), uuid.NewString())

	require.True(t, errors.Is(err, domain.ErrInstanceNotFound))
}

func TestInstanceRepository_ListActiveScopedByWorkspace(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	repo := postgres.NewInstanceRepository(pool)
	wsA, wsB := uuid.NewString(), uuid.NewString()
	firedAt := time.Now().UTC().Truncate(time.Millisecond)

	require.NoError(t, repo.UpsertFiring(ctx, firingInstance(t, wsA, "fp-a", firedAt)))
	require.NoError(t, repo.UpsertFiring(ctx, firingInstance(t, wsB, "fp-b", firedAt)))

	active, err := repo.ListActive(ctx, wsA)
	require.NoError(t, err)
	require.Len(t, active, 1)
	require.Equal(t, "fp-a", active[0].Fingerprint().String())
}
