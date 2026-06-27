package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/namdam97/logmon/backend/internal/slo/domain"
	"github.com/namdam97/logmon/backend/internal/slo/ports"
)

// SnapshotRepository ghi + đọc budget snapshot trên PostgreSQL.
type SnapshotRepository struct {
	pool *pgxpool.Pool
}

var _ ports.SnapshotRepository = (*SnapshotRepository)(nil)

// NewSnapshotRepository tạo repository với pool.
func NewSnapshotRepository(pool *pgxpool.Pool) *SnapshotRepository {
	return &SnapshotRepository{pool: pool}
}

// Save chèn một snapshot budget mới.
func (r *SnapshotRepository) Save(ctx context.Context, snap domain.Snapshot) error {
	const q = `INSERT INTO slo_snapshots
		(slo_id, current_sli, budget_remaining_percent, burn_rate_1h, burn_rate_6h, burn_rate_24h, recorded_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err := dbFrom(ctx, r.pool).Exec(ctx, q,
		snap.SLOID().String(), snap.CurrentSLI(), snap.BudgetRemainingPercent(),
		snap.BurnRate1h(), snap.BurnRate6h(), snap.BurnRate24h(), snap.RecordedAt())
	if err != nil {
		return fmt.Errorf("insert snapshot: %w", err)
	}
	return nil
}

// Latest trả về snapshot mới nhất của một SLO; ErrSnapshotNotFound nếu chưa có.
func (r *SnapshotRepository) Latest(ctx context.Context, sloID domain.SLOID) (domain.Snapshot, error) {
	const q = `SELECT current_sli, budget_remaining_percent, burn_rate_1h, burn_rate_6h, burn_rate_24h, recorded_at
		FROM slo_snapshots WHERE slo_id = $1 ORDER BY recorded_at DESC LIMIT 1`
	var (
		currentSLI, budgetPct, br1h, br6h, br24h float64
		recordedAt                               time.Time
	)
	err := dbFrom(ctx, r.pool).QueryRow(ctx, q, sloID.String()).
		Scan(&currentSLI, &budgetPct, &br1h, &br6h, &br24h, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Snapshot{}, domain.ErrSnapshotNotFound
	}
	if err != nil {
		return domain.Snapshot{}, fmt.Errorf("latest snapshot: %w", err)
	}
	return domain.NewSnapshot(domain.NewSnapshotInput{
		SLOID:                  sloID,
		CurrentSLI:             currentSLI,
		BudgetRemainingPercent: budgetPct,
		BurnRate1h:             br1h,
		BurnRate6h:             br6h,
		BurnRate24h:            br24h,
		RecordedAt:             recordedAt.UTC(),
	}), nil
}
