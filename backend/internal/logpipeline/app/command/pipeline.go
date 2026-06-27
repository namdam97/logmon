// Package command chứa write-side use case của logpipeline BC (CQRS): switch
// mode, cập nhật ILM, retry DLQ. Cấu hình persist là desired-state; orchestration
// thực tế (collector/ES/Kafka) qua ports infra.
package command

import (
	"context"
	"fmt"
	"time"

	"github.com/namdam97/logmon/backend/internal/logpipeline/domain"
	"github.com/namdam97/logmon/backend/internal/logpipeline/ports"
)

// Clock cung cấp thời gian hiện tại — inject để test xác định.
type Clock interface {
	Now() time.Time
}

// PipelineCommands xử lý switch mode + cập nhật ILM.
type PipelineCommands struct {
	configs ports.PipelineConfigRepository
	ilm     ports.ILMApplier
	clock   Clock
}

// NewPipelineCommands tạo service. ilm có thể nil (bỏ qua áp ES — best-effort dev).
func NewPipelineCommands(configs ports.PipelineConfigRepository, ilm ports.ILMApplier, clock Clock) *PipelineCommands {
	return &PipelineCommands{configs: configs, ilm: ilm, clock: clock}
}

// loadOrDefault lấy config hiện có hoặc khởi tạo mặc định cho workspace.
func (s *PipelineCommands) loadOrDefault(ctx context.Context, workspaceID string) (domain.PipelineConfig, error) {
	cfg, err := s.configs.Get(ctx, workspaceID)
	if err == nil {
		return cfg, nil
	}
	// Chưa có cấu hình → mặc định (Mode A + ILM mặc định).
	return domain.DefaultPipelineConfig(workspaceID, s.clock.Now()), nil
}

// SwitchMode đổi chế độ pipeline (A↔B) cho workspace. Lưu desired-state; việc
// orchestrate collector/Kafka thực tế nằm ngoài (IaC/ops) — ghi nhận nợ.
func (s *PipelineCommands) SwitchMode(ctx context.Context, workspaceID, mode, by string) (domain.PipelineConfig, error) {
	m, err := domain.ParseMode(mode)
	if err != nil {
		return domain.PipelineConfig{}, err
	}
	cfg, err := s.loadOrDefault(ctx, workspaceID)
	if err != nil {
		return domain.PipelineConfig{}, err
	}
	updated, err := cfg.WithMode(m, by, s.clock.Now())
	if err != nil {
		return domain.PipelineConfig{}, err
	}
	if err := s.configs.Upsert(ctx, updated); err != nil {
		return domain.PipelineConfig{}, fmt.Errorf("upsert pipeline config: %w", err)
	}
	return updated, nil
}

// UpdateILMInput là dữ liệu cập nhật ILM policy.
type UpdateILMInput struct {
	HotDays    int
	WarmDays   int
	DeleteDays int
}

// UpdateILM validate + áp ILM mới: áp lên ES trước (nếu có applier), sau đó
// persist để tránh drift desired-state khi ES từ chối.
func (s *PipelineCommands) UpdateILM(ctx context.Context, workspaceID, namespace string, in UpdateILMInput, by string) (domain.PipelineConfig, error) {
	cfg, err := s.loadOrDefault(ctx, workspaceID)
	if err != nil {
		return domain.PipelineConfig{}, err
	}
	policy := domain.ILMPolicy{HotDays: in.HotDays, WarmDays: in.WarmDays, DeleteDays: in.DeleteDays}
	updated, err := cfg.WithILM(policy, by, s.clock.Now())
	if err != nil {
		return domain.PipelineConfig{}, err
	}
	if s.ilm != nil {
		if err := s.ilm.Apply(ctx, namespace, policy); err != nil {
			return domain.PipelineConfig{}, fmt.Errorf("apply ilm: %w", err)
		}
	}
	if err := s.configs.Upsert(ctx, updated); err != nil {
		return domain.PipelineConfig{}, fmt.Errorf("upsert pipeline config: %w", err)
	}
	return updated, nil
}
