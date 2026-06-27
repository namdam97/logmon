// Package query chứa read-side use cases của slo BC (CQRS).
package query

import (
	"context"
	"errors"
	"fmt"

	"github.com/namdam97/logmon/backend/internal/slo/domain"
	"github.com/namdam97/logmon/backend/internal/slo/ports"
)

// SLOQueries là read side cô lập workspace cho SLO + budget snapshot.
type SLOQueries struct {
	reader    ports.SLOReader
	snapshots ports.SnapshotRepository
}

// NewSLOQueries tạo read side.
func NewSLOQueries(reader ports.SLOReader, snapshots ports.SnapshotRepository) *SLOQueries {
	return &SLOQueries{reader: reader, snapshots: snapshots}
}

// Get trả về một SLO trong workspace; ErrSLONotFound nếu không thuộc workspace.
func (q *SLOQueries) Get(ctx context.Context, workspaceID, rawID string) (domain.SLO, error) {
	id, err := domain.NewSLOID(rawID)
	if err != nil {
		return domain.SLO{}, err
	}
	s, err := q.reader.ByID(ctx, id)
	if err != nil {
		return domain.SLO{}, err
	}
	if s.WorkspaceID() != workspaceID {
		return domain.SLO{}, domain.ErrSLONotFound
	}
	return s, nil
}

// List trả về mọi SLO của workspace.
func (q *SLOQueries) List(ctx context.Context, workspaceID string) ([]domain.SLO, error) {
	return q.reader.List(ctx, workspaceID)
}

// BudgetView gom SLO + snapshot mới nhất (read model cho /slos/:id/budget).
type BudgetView struct {
	SLO      domain.SLO
	Snapshot domain.Snapshot
	HasData  bool
}

// Budget trả về SLO + snapshot budget mới nhất. HasData=false nếu chưa có snapshot.
func (q *SLOQueries) Budget(ctx context.Context, workspaceID, rawID string) (BudgetView, error) {
	s, err := q.Get(ctx, workspaceID, rawID)
	if err != nil {
		return BudgetView{}, err
	}
	snap, err := q.snapshots.Latest(ctx, s.ID())
	if err != nil {
		if errors.Is(err, domain.ErrSnapshotNotFound) {
			return BudgetView{SLO: s, HasData: false}, nil
		}
		return BudgetView{}, fmt.Errorf("latest snapshot: %w", err)
	}
	return BudgetView{SLO: s, Snapshot: snap, HasData: true}, nil
}

// Compliance trả về tổng quan budget mọi SLO của workspace (tolerant snapshot trống).
func (q *SLOQueries) Compliance(ctx context.Context, workspaceID string) ([]BudgetView, error) {
	slos, err := q.reader.List(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]BudgetView, 0, len(slos))
	for _, s := range slos {
		snap, err := q.snapshots.Latest(ctx, s.ID())
		switch {
		case err == nil:
			out = append(out, BudgetView{SLO: s, Snapshot: snap, HasData: true})
		case errors.Is(err, domain.ErrSnapshotNotFound):
			out = append(out, BudgetView{SLO: s, HasData: false})
		default:
			return nil, fmt.Errorf("latest snapshot: %w", err)
		}
	}
	return out, nil
}
