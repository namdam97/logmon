//go:build integration

// Integration test cho Update/Delete/MarkSynced của RuleRepository — cần Postgres
// thật (DATABASE_URL) đã áp migrations. Chạy: make test-integration.
package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/alerting/adapters/postgres"
	"github.com/namdam97/logmon/backend/internal/alerting/domain"
)

func TestRuleRepository_UpdatePersists(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	h := newHandler(pool)
	created, err := h.Handle(ctx, validInput())
	require.NoError(t, err)

	repo := postgres.NewRuleRepository(pool)
	updated, err := created.Update(domain.UpdateInput{
		Name:        "HighErrorRate", // giữ tên
		Expression:  "up == 1",
		Service:     "logmon-api-v2",
		ForDuration: 7 * time.Minute,
		Severity:    domain.SeverityWarning,
		Labels:      map[string]string{"team": "sre"},
		Annotations: map[string]string{domain.AnnotationSummary: "s2", domain.AnnotationRunbookURL: "u2"},
	}, time.Now().UTC())
	require.NoError(t, err)
	require.NoError(t, repo.Update(ctx, updated))

	got, err := repo.ByID(ctx, created.ID())
	require.NoError(t, err)
	require.Equal(t, "up == 1", got.Expression())
	require.Equal(t, "logmon-api-v2", got.Service())
	require.Equal(t, 7*time.Minute, got.ForDuration())
	require.Equal(t, domain.SeverityWarning.String(), got.Severity().String())
	require.Equal(t, domain.SyncPending, got.SyncStatus())
}

func TestRuleRepository_UpdateMissingReturnsNotFound(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	h := newHandler(pool)
	created, err := h.Handle(ctx, validInput())
	require.NoError(t, err)

	repo := postgres.NewRuleRepository(pool)
	// Xoá rồi update → ErrRuleNotFound.
	require.NoError(t, repo.Delete(ctx, created.ID()))
	err = repo.Update(ctx, created)
	require.ErrorIs(t, err, domain.ErrRuleNotFound)
}

func TestRuleRepository_Delete(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	h := newHandler(pool)
	created, err := h.Handle(ctx, validInput())
	require.NoError(t, err)

	repo := postgres.NewRuleRepository(pool)
	require.NoError(t, repo.Delete(ctx, created.ID()))
	require.Equal(t, 0, count(t, pool, "SELECT count(*) FROM alert_rules WHERE id = $1", created.ID().String()))

	// Xoá lại → ErrRuleNotFound.
	require.ErrorIs(t, repo.Delete(ctx, created.ID()), domain.ErrRuleNotFound)
}

func TestRuleRepository_MarkSyncedAndError(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	h := newHandler(pool)
	created, err := h.Handle(ctx, validInput())
	require.NoError(t, err)

	repo := postgres.NewRuleRepository(pool)
	now := time.Now().UTC()

	require.NoError(t, repo.MarkSynced(ctx, now))
	got, err := repo.ByID(ctx, created.ID())
	require.NoError(t, err)
	require.Equal(t, domain.SyncSynced, got.SyncStatus())
	require.Empty(t, got.SyncError())

	require.NoError(t, repo.MarkSyncError(ctx, "reload prometheus: 500", now))
	got, err = repo.ByID(ctx, created.ID())
	require.NoError(t, err)
	require.Equal(t, domain.SyncError, got.SyncStatus())
	require.Equal(t, "reload prometheus: 500", got.SyncError())
}
