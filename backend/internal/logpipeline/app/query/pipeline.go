package query

import (
	"context"
	"fmt"
	"time"

	"github.com/namdam97/logmon/backend/internal/logpipeline/domain"
	"github.com/namdam97/logmon/backend/internal/logpipeline/ports"
)

// StatusView là read model cho GET /pipeline/status.
type StatusView struct {
	Mode        string
	Health      domain.HealthStatus
	DataStreams int
	UpdatedAt   time.Time
}

// PipelineQueries cung cấp read-side cho pipeline management.
type PipelineQueries struct {
	configs     ports.PipelineConfigRepository
	dlq         ports.DLQReader
	health      ports.PipelineHealth
	datastreams ports.DataStreamReader
	clock       clock
}

type clock interface{ Now() time.Time }

// NewPipelineQueries tạo service. health/datastreams có thể nil (degrade an toàn).
func NewPipelineQueries(configs ports.PipelineConfigRepository, dlq ports.DLQReader, health ports.PipelineHealth, datastreams ports.DataStreamReader, c clock) *PipelineQueries {
	return &PipelineQueries{configs: configs, dlq: dlq, health: health, datastreams: datastreams, clock: c}
}

// GetConfig trả cấu hình pipeline (mặc định nếu chưa có).
func (q *PipelineQueries) GetConfig(ctx context.Context, workspaceID string) (domain.PipelineConfig, error) {
	cfg, err := q.configs.Get(ctx, workspaceID)
	if err == nil {
		return cfg, nil
	}
	return domain.DefaultPipelineConfig(workspaceID, q.clock.Now()), nil
}

// Status tổng hợp mode + health + số data stream cho workspace.
func (q *PipelineQueries) Status(ctx context.Context, workspaceID, namespace string) (StatusView, error) {
	cfg, err := q.GetConfig(ctx, workspaceID)
	if err != nil {
		return StatusView{}, err
	}
	view := StatusView{Mode: cfg.Mode().String(), UpdatedAt: cfg.UpdatedAt()}
	if q.health != nil {
		view.Health = q.health.Check(ctx)
	}
	if q.datastreams != nil {
		if stats, err := q.datastreams.Stats(ctx, namespace); err == nil {
			view.DataStreams = len(stats)
		}
	}
	return view, nil
}

// ListDLQ trả entries + đếm theo trạng thái.
func (q *PipelineQueries) ListDLQ(ctx context.Context, workspaceID, statusFilter string, limit int) ([]domain.DLQEntry, map[string]int, error) {
	entries, err := q.dlq.List(ctx, workspaceID, statusFilter, limit)
	if err != nil {
		return nil, nil, fmt.Errorf("list dlq: %w", err)
	}
	counts, err := q.dlq.CountByStatus(ctx, workspaceID)
	if err != nil {
		return nil, nil, fmt.Errorf("count dlq: %w", err)
	}
	return entries, counts, nil
}

// DataStreams trả thống kê data stream của workspace (rỗng nếu không có reader).
func (q *PipelineQueries) DataStreams(ctx context.Context, namespace string) ([]domain.DataStreamStat, error) {
	if q.datastreams == nil {
		return nil, nil
	}
	return q.datastreams.Stats(ctx, namespace)
}
