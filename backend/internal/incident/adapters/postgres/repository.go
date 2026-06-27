package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/namdam97/logmon/backend/internal/incident/domain"
	"github.com/namdam97/logmon/backend/internal/incident/ports"
)

const incidentColumns = `id, workspace_id, title, description, service, severity,
	status, source, source_ref, assignee, created_at, updated_at,
	assigned_at, resolved_at, closed_at`

// _activeStatuses là tập trạng thái "đang active" (dùng cho ListActive + dedup).
var _activeStatuses = []string{
	domain.StatusOpen.String(),
	domain.StatusTriaged.String(),
	domain.StatusAssigned.String(),
	domain.StatusMitigating.String(),
}

// IncidentRepository lưu trữ + đọc incident trên PostgreSQL.
type IncidentRepository struct {
	pool *pgxpool.Pool
}

var (
	_ ports.IncidentRepository = (*IncidentRepository)(nil)
	_ ports.IncidentReader     = (*IncidentRepository)(nil)
	_ ports.UnackedReader      = (*IncidentRepository)(nil)
)

// _unackedStatuses là incident active nhưng CHƯA được ack (chưa Assigned) — đầu
// vào escalation runner.
var _unackedStatuses = []string{
	domain.StatusOpen.String(),
	domain.StatusTriaged.String(),
}

// NewIncidentRepository tạo repository với pool.
func NewIncidentRepository(pool *pgxpool.Pool) *IncidentRepository {
	return &IncidentRepository{pool: pool}
}

// Save chèn incident mới (trong tx của ctx).
func (r *IncidentRepository) Save(ctx context.Context, inc domain.Incident) error {
	const q = `INSERT INTO incidents
		(id, workspace_id, title, description, service, severity, status, source,
		 source_ref, assignee, created_at, updated_at, assigned_at, resolved_at, closed_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`
	_, err := dbFrom(ctx, r.pool).Exec(ctx, q,
		inc.ID().String(), inc.WorkspaceID(), inc.Title(), inc.Description(), inc.Service(),
		nullableSeverity(inc.Severity()), inc.Status().String(), inc.Source().String(),
		inc.SourceRef(), inc.Assignee(), inc.CreatedAt(), inc.UpdatedAt(),
		inc.AssignedAt(), inc.ResolvedAt(), inc.ClosedAt())
	if err != nil {
		return fmt.Errorf("insert incident: %w", err)
	}
	return nil
}

// Update ghi đè incident theo id (trong tx của ctx).
func (r *IncidentRepository) Update(ctx context.Context, inc domain.Incident) error {
	const q = `UPDATE incidents SET
		title=$2, description=$3, service=$4, severity=$5, status=$6, source_ref=$7,
		assignee=$8, updated_at=$9, assigned_at=$10, resolved_at=$11, closed_at=$12
		WHERE id=$1`
	tag, err := dbFrom(ctx, r.pool).Exec(ctx, q,
		inc.ID().String(), inc.Title(), inc.Description(), inc.Service(),
		nullableSeverity(inc.Severity()), inc.Status().String(), inc.SourceRef(),
		inc.Assignee(), inc.UpdatedAt(), inc.AssignedAt(), inc.ResolvedAt(), inc.ClosedAt())
	if err != nil {
		return fmt.Errorf("update incident: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrIncidentNotFound
	}
	return nil
}

// ByID đọc incident theo id; ErrIncidentNotFound nếu không có.
func (r *IncidentRepository) ByID(ctx context.Context, id domain.IncidentID) (domain.Incident, error) {
	const q = `SELECT ` + incidentColumns + ` FROM incidents WHERE id = $1`
	inc, err := scanIncident(dbFrom(ctx, r.pool).QueryRow(ctx, q, id.String()))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Incident{}, domain.ErrIncidentNotFound
	}
	return inc, err
}

// List đọc mọi incident của workspace (mới nhất trước).
func (r *IncidentRepository) List(ctx context.Context, workspaceID string) ([]domain.Incident, error) {
	const q = `SELECT ` + incidentColumns + ` FROM incidents
		WHERE workspace_id = $1 ORDER BY created_at DESC`
	return r.queryIncidents(ctx, q, workspaceID)
}

// ListActive đọc incident đang active của workspace (incident board).
func (r *IncidentRepository) ListActive(ctx context.Context, workspaceID string) ([]domain.Incident, error) {
	const q = `SELECT ` + incidentColumns + ` FROM incidents
		WHERE workspace_id = $1 AND status = ANY($2) ORDER BY created_at DESC`
	return r.queryIncidents(ctx, q, workspaceID, _activeStatuses)
}

// ActiveBySourceRef tìm incident active cùng source+ref; ErrIncidentNotFound nếu không có.
func (r *IncidentRepository) ActiveBySourceRef(ctx context.Context, workspaceID string, source domain.Source, sourceRef string) (domain.Incident, error) {
	const q = `SELECT ` + incidentColumns + ` FROM incidents
		WHERE workspace_id = $1 AND source = $2 AND source_ref = $3 AND status = ANY($4)
		ORDER BY created_at DESC LIMIT 1`
	inc, err := scanIncident(dbFrom(ctx, r.pool).QueryRow(ctx, q, workspaceID, source.String(), sourceRef, _activeStatuses))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Incident{}, domain.ErrIncidentNotFound
	}
	return inc, err
}

// ListUnacked đọc incident chưa ack (status open/triaged) trên mọi workspace —
// escalation runner quét toàn cục (cũ nhất trước để escalate đúng thứ tự).
func (r *IncidentRepository) ListUnacked(ctx context.Context) ([]domain.Incident, error) {
	const q = `SELECT ` + incidentColumns + ` FROM incidents
		WHERE status = ANY($1) ORDER BY created_at ASC`
	return r.queryIncidents(ctx, q, _unackedStatuses)
}

func (r *IncidentRepository) queryIncidents(ctx context.Context, q string, args ...any) ([]domain.Incident, error) {
	rows, err := dbFrom(ctx, r.pool).Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query incidents: %w", err)
	}
	defer rows.Close()

	var incidents []domain.Incident
	for rows.Next() {
		inc, err := scanIncident(rows)
		if err != nil {
			return nil, err
		}
		incidents = append(incidents, inc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate incidents: %w", err)
	}
	return incidents, nil
}

type scanRow interface {
	Scan(dest ...any) error
}

func scanIncident(row scanRow) (domain.Incident, error) {
	var (
		rawID, workspaceID, title, description, service string
		severity                                        *string
		status, source, sourceRef, assignee             string
		createdAt, updatedAt                            time.Time
		assignedAt, resolvedAt, closedAt                *time.Time
	)
	if err := row.Scan(&rawID, &workspaceID, &title, &description, &service, &severity,
		&status, &source, &sourceRef, &assignee, &createdAt, &updatedAt,
		&assignedAt, &resolvedAt, &closedAt); err != nil {
		return domain.Incident{}, err
	}

	id, err := domain.NewIncidentID(rawID)
	if err != nil {
		return domain.Incident{}, fmt.Errorf("reconstruct id: %w", err)
	}
	sev, err := reconstructSeverity(severity)
	if err != nil {
		return domain.Incident{}, err
	}
	st, err := domain.NewStatus(status)
	if err != nil {
		return domain.Incident{}, fmt.Errorf("reconstruct status: %w", err)
	}
	src, err := domain.NewSource(source)
	if err != nil {
		return domain.Incident{}, fmt.Errorf("reconstruct source: %w", err)
	}

	return domain.Reconstruct(domain.ReconstructInput{
		ID:          id,
		WorkspaceID: workspaceID,
		Title:       title,
		Description: description,
		Service:     service,
		Severity:    sev,
		Status:      st,
		Source:      src,
		SourceRef:   sourceRef,
		Assignee:    assignee,
		CreatedAt:   createdAt.UTC(),
		UpdatedAt:   updatedAt.UTC(),
		AssignedAt:  utcPtr(assignedAt),
		ResolvedAt:  utcPtr(resolvedAt),
		ClosedAt:    utcPtr(closedAt),
	}), nil
}

func reconstructSeverity(raw *string) (domain.Severity, error) {
	if raw == nil || *raw == "" {
		return domain.Severity{}, nil
	}
	sev, err := domain.NewSeverity(*raw)
	if err != nil {
		return domain.Severity{}, fmt.Errorf("reconstruct severity: %w", err)
	}
	return sev, nil
}

// nullableSeverity map severity chưa phân loại (zero) → NULL.
func nullableSeverity(s domain.Severity) *string {
	if s.IsZero() {
		return nil
	}
	v := s.String()
	return &v
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}
