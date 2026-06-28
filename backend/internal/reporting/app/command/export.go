package command

import (
	"context"
	"fmt"

	"github.com/namdam97/logmon/backend/internal/reporting/domain"
	"github.com/namdam97/logmon/backend/internal/reporting/ports"
)

// ExportService tạo export job bất đồng bộ (worker xử lý sau).
type ExportService struct {
	repo  ports.ExportJobRepository
	ids   ports.IDGenerator
	clock ports.Clock
}

// NewExportService tạo service.
func NewExportService(repo ports.ExportJobRepository, ids ports.IDGenerator, clock ports.Clock) *ExportService {
	return &ExportService{repo: repo, ids: ids, clock: clock}
}

// CreateExportInput là dữ liệu tạo export job.
type CreateExportInput struct {
	WorkspaceID string
	UserID      string
	ExportType  string
	Format      string
	QueryParams map[string]any
}

// Create tạo job pending; worker sẽ xử lý + upload S3 + cấp signed URL.
func (s *ExportService) Create(ctx context.Context, in CreateExportInput) (domain.ExportJob, error) {
	et, err := domain.ParseExportType(in.ExportType)
	if err != nil {
		return domain.ExportJob{}, err
	}
	format, err := domain.ParseReportFormat(in.Format)
	if err != nil {
		return domain.ExportJob{}, err
	}
	job, err := domain.NewExportJob(domain.NewJobInput{
		ID: s.ids.NewID(), WorkspaceID: in.WorkspaceID, UserID: in.UserID, ExportType: et,
		QueryParams: in.QueryParams, Format: format, Now: s.clock.Now(),
	})
	if err != nil {
		return domain.ExportJob{}, err
	}
	if err := s.repo.Save(ctx, job); err != nil {
		return domain.ExportJob{}, fmt.Errorf("save export job: %w", err)
	}
	return job, nil
}
