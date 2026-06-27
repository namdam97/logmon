package ports

import (
	"context"
	"time"

	"github.com/namdam97/logmon/backend/internal/logpipeline/domain"
)

// --- Persistence (source of truth cho desired state) ---

// PipelineConfigRepository lưu cấu hình pipeline (mode + ILM) per workspace.
type PipelineConfigRepository interface {
	// Get trả cấu hình; domain.ErrPipelineConfigNotFound nếu chưa có.
	Get(ctx context.Context, workspaceID string) (domain.PipelineConfig, error)
	// Upsert chèn/cập nhật cấu hình (unique theo workspace_id).
	Upsert(ctx context.Context, c domain.PipelineConfig) error
}

// DLQRepository quản lý trạng thái DLQ entry (write side).
type DLQRepository interface {
	// ByID lấy entry trong workspace; domain.ErrDLQEntryNotFound nếu không có.
	ByID(ctx context.Context, workspaceID string, id int64) (domain.DLQEntry, error)
	// UpdateStatus cập nhật trạng thái + retry_count + retried_at của entry.
	UpdateStatus(ctx context.Context, workspaceID string, id int64, status domain.DLQStatus, retryCount int, retriedAt *time.Time) error
}

// DLQReader đọc DLQ (read side): list + đếm theo trạng thái.
type DLQReader interface {
	// List trả entries của workspace (statusFilter rỗng = mọi trạng thái).
	List(ctx context.Context, workspaceID, statusFilter string, limit int) ([]domain.DLQEntry, error)
	// CountByStatus trả map status→count cho workspace.
	CountByStatus(ctx context.Context, workspaceID string) (map[string]int, error)
}

// --- Orchestration ngoài (infra adapters) ---

// ILMApplier áp ILM policy lên Elasticsearch (PUT _ilm/policy). namespace = slug
// workspace để policy/retention tách theo tenant (doc_v2/03 §4).
type ILMApplier interface {
	Apply(ctx context.Context, namespace string, p domain.ILMPolicy) error
}

// DLQReplayer re-publish một entry sau review (Mode A: re-index ES; Mode B: Kafka).
type DLQReplayer interface {
	Replay(ctx context.Context, e domain.DLQEntry) error
}

// PipelineHealth kiểm tra tình trạng collector/ES/Kafka cho /status.
type PipelineHealth interface {
	Check(ctx context.Context) domain.HealthStatus
}

// DataStreamReader đọc thống kê data stream từ ES cho /datastreams.
type DataStreamReader interface {
	Stats(ctx context.Context, namespace string) ([]domain.DataStreamStat, error)
}
