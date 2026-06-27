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

// Persistence cho postmortem + action item (doc_v2/06 §1.5). Postmortem 1-1 với
// incident (unique incident_id).

const postmortemColumns = `id, incident_id, workspace_id, status, root_cause,
	impact_duration_seconds, impact_error_count, impact_budget_percent, impact_summary,
	timeline_summary, lessons_learned, created_at, updated_at, published_at`

// PostmortemRepo lưu trữ + đọc postmortem.
type PostmortemRepo struct {
	pool *pgxpool.Pool
}

var (
	_ ports.PostmortemRepository = (*PostmortemRepo)(nil)
	_ ports.PostmortemReader     = (*PostmortemRepo)(nil)
)

// NewPostmortemRepo tạo repo với pool.
func NewPostmortemRepo(pool *pgxpool.Pool) *PostmortemRepo { return &PostmortemRepo{pool: pool} }

// Save chèn postmortem mới.
func (r *PostmortemRepo) Save(ctx context.Context, pm domain.Postmortem) error {
	const q = `INSERT INTO postmortems
		(id, incident_id, workspace_id, status, root_cause, impact_duration_seconds,
		 impact_error_count, impact_budget_percent, impact_summary, timeline_summary,
		 lessons_learned, created_at, updated_at, published_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`
	im := pm.Impact()
	_, err := dbFrom(ctx, r.pool).Exec(ctx, q,
		pm.ID().String(), pm.IncidentID().String(), pm.WorkspaceID(), pm.Status().String(),
		pm.RootCause(), im.DurationSeconds, im.ErrorCount, im.BudgetConsumedPercent, im.Summary,
		pm.TimelineSummary(), pm.LessonsLearned(), pm.CreatedAt(), pm.UpdatedAt(), pm.PublishedAt())
	if err != nil {
		return fmt.Errorf("insert postmortem: %w", err)
	}
	return nil
}

// Update ghi đè postmortem theo id.
func (r *PostmortemRepo) Update(ctx context.Context, pm domain.Postmortem) error {
	const q = `UPDATE postmortems SET
		status=$2, root_cause=$3, impact_duration_seconds=$4, impact_error_count=$5,
		impact_budget_percent=$6, impact_summary=$7, timeline_summary=$8,
		lessons_learned=$9, updated_at=$10, published_at=$11
		WHERE id=$1`
	im := pm.Impact()
	tag, err := dbFrom(ctx, r.pool).Exec(ctx, q,
		pm.ID().String(), pm.Status().String(), pm.RootCause(), im.DurationSeconds,
		im.ErrorCount, im.BudgetConsumedPercent, im.Summary, pm.TimelineSummary(),
		pm.LessonsLearned(), pm.UpdatedAt(), pm.PublishedAt())
	if err != nil {
		return fmt.Errorf("update postmortem: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrPostmortemNotFound
	}
	return nil
}

// ByIncident đọc postmortem của incident; ErrPostmortemNotFound nếu chưa có.
func (r *PostmortemRepo) ByIncident(ctx context.Context, incidentID domain.IncidentID) (domain.Postmortem, error) {
	const q = `SELECT ` + postmortemColumns + ` FROM postmortems WHERE incident_id = $1`
	pm, err := scanPostmortem(dbFrom(ctx, r.pool).QueryRow(ctx, q, incidentID.String()))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Postmortem{}, domain.ErrPostmortemNotFound
	}
	return pm, err
}

func scanPostmortem(row scanRow) (domain.Postmortem, error) {
	var (
		id, incidentID, workspaceID, status string
		rootCause, impactSummary            string
		timelineSummary, lessonsLearned     string
		durationSeconds, errorCount         int64
		budgetPercent                       float64
		createdAt, updatedAt                time.Time
		publishedAt                         *time.Time
	)
	if err := row.Scan(&id, &incidentID, &workspaceID, &status, &rootCause,
		&durationSeconds, &errorCount, &budgetPercent, &impactSummary,
		&timelineSummary, &lessonsLearned, &createdAt, &updatedAt, &publishedAt); err != nil {
		return domain.Postmortem{}, err
	}
	return domain.ReconstructPostmortem(domain.ReconstructPostmortemInput{
		ID:          id,
		IncidentID:  incidentID,
		WorkspaceID: workspaceID,
		Status:      status,
		RootCause:   rootCause,
		Impact: domain.Impact{
			DurationSeconds:       durationSeconds,
			ErrorCount:            errorCount,
			BudgetConsumedPercent: budgetPercent,
			Summary:               impactSummary,
		},
		TimelineSummary: timelineSummary,
		LessonsLearned:  lessonsLearned,
		CreatedAt:       createdAt.UTC(),
		UpdatedAt:       updatedAt.UTC(),
		PublishedAt:     utcPtr(publishedAt),
	}), nil
}

// ActionItemRepo lưu trữ + đọc action item.
type ActionItemRepo struct {
	pool *pgxpool.Pool
}

var (
	_ ports.ActionItemRepository = (*ActionItemRepo)(nil)
	_ ports.ActionItemReader     = (*ActionItemRepo)(nil)
)

// NewActionItemRepo tạo repo với pool.
func NewActionItemRepo(pool *pgxpool.Pool) *ActionItemRepo { return &ActionItemRepo{pool: pool} }

const actionItemColumns = `id, postmortem_id, title, assignee, due_date, status,
	created_at, updated_at, completed_at`

// Save chèn action item mới.
func (r *ActionItemRepo) Save(ctx context.Context, item domain.ActionItem) error {
	const q = `INSERT INTO action_items
		(id, postmortem_id, title, assignee, due_date, status, created_at, updated_at, completed_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`
	_, err := dbFrom(ctx, r.pool).Exec(ctx, q,
		item.ID(), item.PostmortemID().String(), item.Title(), item.Assignee(),
		item.DueDate(), item.Status().String(), item.CreatedAt(), item.UpdatedAt(), item.CompletedAt())
	if err != nil {
		return fmt.Errorf("insert action item: %w", err)
	}
	return nil
}

// Update ghi đè action item theo id.
func (r *ActionItemRepo) Update(ctx context.Context, item domain.ActionItem) error {
	const q = `UPDATE action_items SET
		title=$2, assignee=$3, due_date=$4, status=$5, updated_at=$6, completed_at=$7
		WHERE id=$1`
	tag, err := dbFrom(ctx, r.pool).Exec(ctx, q,
		item.ID(), item.Title(), item.Assignee(), item.DueDate(),
		item.Status().String(), item.UpdatedAt(), item.CompletedAt())
	if err != nil {
		return fmt.Errorf("update action item: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrActionItemNotFound
	}
	return nil
}

// ByID đọc action item theo id; ErrActionItemNotFound nếu không có.
func (r *ActionItemRepo) ByID(ctx context.Context, id string) (domain.ActionItem, error) {
	const q = `SELECT ` + actionItemColumns + ` FROM action_items WHERE id = $1`
	item, err := scanActionItem(dbFrom(ctx, r.pool).QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ActionItem{}, domain.ErrActionItemNotFound
	}
	return item, err
}

// ListByPostmortem đọc action item của một postmortem (cũ nhất trước).
func (r *ActionItemRepo) ListByPostmortem(ctx context.Context, postmortemID domain.PostmortemID) ([]domain.ActionItem, error) {
	const q = `SELECT ` + actionItemColumns + ` FROM action_items
		WHERE postmortem_id = $1 ORDER BY created_at ASC`
	rows, err := dbFrom(ctx, r.pool).Query(ctx, q, postmortemID.String())
	if err != nil {
		return nil, fmt.Errorf("query action items: %w", err)
	}
	defer rows.Close()

	var out []domain.ActionItem
	for rows.Next() {
		item, err := scanActionItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate action items: %w", err)
	}
	return out, nil
}

func scanActionItem(row scanRow) (domain.ActionItem, error) {
	var (
		id, postmortemID, title, assignee, status string
		dueDate, completedAt                      *time.Time
		createdAt, updatedAt                      time.Time
	)
	if err := row.Scan(&id, &postmortemID, &title, &assignee, &dueDate, &status,
		&createdAt, &updatedAt, &completedAt); err != nil {
		return domain.ActionItem{}, err
	}
	created := createdAt.UTC()
	updated := updatedAt.UTC()
	return domain.ReconstructActionItem(id, postmortemID, title, assignee, status,
		utcPtr(dueDate), &created, &updated, utcPtr(completedAt)), nil
}
