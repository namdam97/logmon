package domain

import (
	"errors"
	"strings"
	"time"

	apperrors "github.com/namdam97/logmon/backend/internal/shared/errors"
)

// Domain sentinel errors cho pipeline management.
var (
	// ErrPipelineConfigNotFound: chưa có cấu hình pipeline cho workspace.
	ErrPipelineConfigNotFound = errors.New("pipeline config not found")
	// ErrDLQEntryNotFound: không có DLQ entry theo id.
	ErrDLQEntryNotFound = errors.New("dlq entry not found")
	// ErrDLQNotRetryable: entry không ở trạng thái pending (đã retried/discarded).
	ErrDLQNotRetryable = errors.New("dlq entry not retryable")
)

// Mode là chế độ pipeline log: A (OTLP trực tiếp) hoặc B (Kafka buffer) — ADR-018/027.
type Mode int

// Enum bắt đầu từ 1 (zero value không hợp lệ).
const (
	// ModeA: OTLP gRPC trực tiếp → gateway → ES (mặc định, <10K logs/s).
	ModeA Mode = iota + 1
	// ModeB: thêm Kafka buffer (>10K logs/s, replay được).
	ModeB
)

// String trả về biểu diễn 1 ký tự ('A'/'B') khớp cột DB.
func (m Mode) String() string {
	switch m {
	case ModeA:
		return "A"
	case ModeB:
		return "B"
	default:
		return "?"
	}
}

// ParseMode chuyển chuỗi 'A'/'B' (case-insensitive) thành Mode.
func ParseMode(raw string) (Mode, error) {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "A":
		return ModeA, nil
	case "B":
		return ModeB, nil
	default:
		return 0, apperrors.NewValidationError("mode", "must be 'A' or 'B'")
	}
}

// ILMPolicy là chính sách vòng đời index ES (hot→warm→delete theo ngày) —
// doc_v2/03 §4.2. Bất biến; validate hot≥1 < warm < delete.
type ILMPolicy struct {
	HotDays    int
	WarmDays   int
	DeleteDays int
}

// DefaultILMPolicy là mặc định doc_v2/03: hot 7d → warm 30d → delete 90d.
func DefaultILMPolicy() ILMPolicy {
	return ILMPolicy{HotDays: 7, WarmDays: 30, DeleteDays: 90}
}

func (p ILMPolicy) validate() error {
	if p.HotDays < 1 {
		return apperrors.NewValidationError("hotDays", "must be at least 1")
	}
	if p.WarmDays <= p.HotDays {
		return apperrors.NewValidationError("warmDays", "must be greater than hotDays")
	}
	if p.DeleteDays <= p.WarmDays {
		return apperrors.NewValidationError("deleteDays", "must be greater than warmDays")
	}
	return nil
}

// PipelineConfig là aggregate cấu hình pipeline của một workspace (mode + ILM).
// Là desired-state — backend orchestrate collector/ES theo giá trị này.
type PipelineConfig struct {
	workspaceID string
	mode        Mode
	ilm         ILMPolicy
	updatedAt   time.Time
	updatedBy   string
}

// DefaultPipelineConfig tạo cấu hình mặc định (Mode A + ILM mặc định).
func DefaultPipelineConfig(workspaceID string, now time.Time) PipelineConfig {
	return PipelineConfig{
		workspaceID: workspaceID,
		mode:        ModeA,
		ilm:         DefaultILMPolicy(),
		updatedAt:   now,
	}
}

// ReconstructPipelineConfig dựng lại từ storage — KHÔNG validate lại.
func ReconstructPipelineConfig(workspaceID string, mode Mode, ilm ILMPolicy, updatedAt time.Time, updatedBy string) PipelineConfig {
	return PipelineConfig{workspaceID: workspaceID, mode: mode, ilm: ilm, updatedAt: updatedAt, updatedBy: updatedBy}
}

// WorkspaceID trả về workspace của cấu hình.
func (c PipelineConfig) WorkspaceID() string { return c.workspaceID }

// Mode trả về chế độ hiện tại.
func (c PipelineConfig) Mode() Mode { return c.mode }

// ILM trả về chính sách ILM hiện tại.
func (c PipelineConfig) ILM() ILMPolicy { return c.ilm }

// UpdatedAt trả về thời điểm cập nhật gần nhất (UTC).
func (c PipelineConfig) UpdatedAt() time.Time { return c.updatedAt }

// UpdatedBy trả về user thực hiện cập nhật gần nhất.
func (c PipelineConfig) UpdatedBy() string { return c.updatedBy }

// WithMode trả về bản sao với mode mới (bất biến).
func (c PipelineConfig) WithMode(mode Mode, by string, now time.Time) (PipelineConfig, error) {
	if mode != ModeA && mode != ModeB {
		return PipelineConfig{}, apperrors.NewValidationError("mode", "invalid mode")
	}
	cp := c
	cp.mode = mode
	cp.updatedBy = by
	cp.updatedAt = now
	return cp, nil
}

// WithILM trả về bản sao với chính sách ILM mới đã validate (bất biến).
func (c PipelineConfig) WithILM(ilm ILMPolicy, by string, now time.Time) (PipelineConfig, error) {
	if err := ilm.validate(); err != nil {
		return PipelineConfig{}, err
	}
	cp := c
	cp.ilm = ilm
	cp.updatedBy = by
	cp.updatedAt = now
	return cp, nil
}
