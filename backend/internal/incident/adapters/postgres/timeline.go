package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/namdam97/logmon/backend/internal/incident/domain"
	"github.com/namdam97/logmon/backend/internal/incident/ports"
)

const timelineColumns = `id, incident_id, kind, from_status, to_status, actor, note, at`

// TimelineRepository lưu trữ + đọc mục timeline incident trên PostgreSQL.
type TimelineRepository struct {
	pool *pgxpool.Pool
}

var _ ports.TimelineRepository = (*TimelineRepository)(nil)

// NewTimelineRepository tạo repository với pool.
func NewTimelineRepository(pool *pgxpool.Pool) *TimelineRepository {
	return &TimelineRepository{pool: pool}
}

// Append chèn một mục timeline (trong tx của ctx).
func (r *TimelineRepository) Append(ctx context.Context, e domain.TimelineEntry) error {
	const q = `INSERT INTO incident_timeline
		(id, incident_id, kind, from_status, to_status, actor, note, at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`
	_, err := dbFrom(ctx, r.pool).Exec(ctx, q,
		e.ID(), e.IncidentID(), string(e.Kind()),
		nullableStatus(e.FromStatus()), nullableStatus(e.ToStatus()),
		e.Actor(), e.Note(), e.At())
	if err != nil {
		return fmt.Errorf("insert timeline entry: %w", err)
	}
	return nil
}

// List đọc dòng thời gian của một incident (cũ nhất trước — kể chuyện theo thứ tự).
func (r *TimelineRepository) List(ctx context.Context, incidentID domain.IncidentID) ([]domain.TimelineEntry, error) {
	const q = `SELECT ` + timelineColumns + ` FROM incident_timeline
		WHERE incident_id = $1 ORDER BY at ASC, id ASC`
	rows, err := dbFrom(ctx, r.pool).Query(ctx, q, incidentID.String())
	if err != nil {
		return nil, fmt.Errorf("query timeline: %w", err)
	}
	defer rows.Close()

	var entries []domain.TimelineEntry
	for rows.Next() {
		var (
			id, incID, kind     string
			fromStatus, toState *string
			actor, note         string
			at                  time.Time
		)
		if err := rows.Scan(&id, &incID, &kind, &fromStatus, &toState, &actor, &note, &at); err != nil {
			return nil, fmt.Errorf("scan timeline entry: %w", err)
		}
		entries = append(entries, domain.ReconstructTimelineEntry(domain.NewTimelineEntryInput{
			ID:         id,
			IncidentID: incID,
			Kind:       domain.TimelineKind(kind),
			FromStatus: statusOrZero(fromStatus),
			ToStatus:   statusOrZero(toState),
			Actor:      actor,
			Note:       note,
			At:         at.UTC(),
		}))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate timeline: %w", err)
	}
	return entries, nil
}

// nullableStatus map status zero (vd note không có from) → NULL.
func nullableStatus(s domain.Status) *string {
	if (s == domain.Status{}) {
		return nil
	}
	v := s.String()
	return &v
}

// statusOrZero hydrate status; nil/rỗng/không hợp lệ → zero Status.
func statusOrZero(raw *string) domain.Status {
	if raw == nil || *raw == "" {
		return domain.Status{}
	}
	if s, err := domain.NewStatus(*raw); err == nil {
		return s
	}
	return domain.Status{}
}
