// Package app chứa use case usage BC: tổng hợp usage + ước tính chi phí, quản lý
// quota per workspace.
package app

import (
	"context"
	"fmt"
	"time"

	"github.com/namdam97/logmon/backend/internal/usage/domain"
	"github.com/namdam97/logmon/backend/internal/usage/ports"
)

// _usageWindow là cửa sổ tính ingestion/log count (30 ngày gần nhất).
const _usageWindow = 30 * 24 * time.Hour

// Service cung cấp usage + quota.
type Service struct {
	quotas ports.QuotaRepository
	usage  ports.UsageReader
	clock  ports.Clock
}

// NewService tạo service. usage có thể nil (chưa cấu hình nguồn → usage = 0).
func NewService(quotas ports.QuotaRepository, usage ports.UsageReader, clock ports.Clock) *Service {
	return &Service{quotas: quotas, usage: usage, clock: clock}
}

// GetUsage tổng hợp usage 30 ngày + storage hiện tại + ước tính chi phí.
func (s *Service) GetUsage(ctx context.Context, workspaceID string) (domain.UsageSummary, error) {
	now := s.clock.Now()
	since := now.Add(-_usageWindow)
	summary := domain.UsageSummary{WorkspaceID: workspaceID, PeriodStart: since, PeriodEnd: now}
	if s.usage == nil {
		return summary, nil
	}
	ingestion, err := s.usage.IngestionBytes(ctx, workspaceID, since)
	if err != nil {
		return domain.UsageSummary{}, fmt.Errorf("read ingestion: %w", err)
	}
	storage, err := s.usage.StorageBytes(ctx, workspaceID)
	if err != nil {
		return domain.UsageSummary{}, fmt.Errorf("read storage: %w", err)
	}
	logs, err := s.usage.LogCount(ctx, workspaceID, since)
	if err != nil {
		return domain.UsageSummary{}, fmt.Errorf("read log count: %w", err)
	}
	summary.IngestionBytes = ingestion
	summary.StorageBytes = storage
	summary.LogCount = logs
	summary.EstimatedCostUSD = domain.EstimateCostUSD(ingestion, storage)
	return summary, nil
}

// GetQuota trả quota của workspace (mặc định nếu chưa cấu hình).
func (s *Service) GetQuota(ctx context.Context, workspaceID string) (domain.Quota, error) {
	q, err := s.quotas.Get(ctx, workspaceID)
	if err == nil {
		return q, nil
	}
	return domain.DefaultQuota(workspaceID, s.clock.Now()), nil
}

// SetQuotaInput là dữ liệu cập nhật quota.
type SetQuotaInput struct {
	WorkspaceID             string
	MaxIngestionBytesPerDay int64
	MaxStorageBytes         int64
	RetentionDays           int
}

// SetQuota validate + lưu quota mới.
func (s *Service) SetQuota(ctx context.Context, in SetQuotaInput) (domain.Quota, error) {
	q, err := domain.NewQuota(in.WorkspaceID, in.MaxIngestionBytesPerDay, in.MaxStorageBytes, in.RetentionDays, s.clock.Now())
	if err != nil {
		return domain.Quota{}, err
	}
	if err := s.quotas.Upsert(ctx, q); err != nil {
		return domain.Quota{}, fmt.Errorf("upsert quota: %w", err)
	}
	return q, nil
}
