package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/namdam97/logmon/backend/internal/incident/domain"
	"github.com/namdam97/logmon/backend/internal/incident/ports"
)

// Persistence cho on-call & escalation: schedules, overrides, escalation policies
// (1/workspace), và escalation state (bậc đã thông báo theo incident).

// ScheduleRepo lưu trữ + đọc on-call schedule.
type ScheduleRepo struct {
	pool *pgxpool.Pool
}

var (
	_ ports.ScheduleRepository = (*ScheduleRepo)(nil)
	_ ports.ScheduleReader     = (*ScheduleRepo)(nil)
)

// NewScheduleRepo tạo repo với pool.
func NewScheduleRepo(pool *pgxpool.Pool) *ScheduleRepo { return &ScheduleRepo{pool: pool} }

const scheduleColumns = `id, workspace_id, name, rotation, participants, timezone, anchor, created_at, updated_at`

// Save chèn schedule mới.
func (r *ScheduleRepo) Save(ctx context.Context, s domain.Schedule) error {
	const q = `INSERT INTO oncall_schedules
		(id, workspace_id, name, rotation, participants, timezone, anchor, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`
	_, err := dbFrom(ctx, r.pool).Exec(ctx, q,
		s.ID().String(), s.WorkspaceID(), s.Name(), s.Rotation().String(),
		s.Participants(), s.Timezone(), s.Anchor().UTC(), s.CreatedAt(), s.UpdatedAt())
	if err != nil {
		return fmt.Errorf("insert schedule: %w", err)
	}
	return nil
}

// Update ghi đè schedule theo id.
func (r *ScheduleRepo) Update(ctx context.Context, s domain.Schedule) error {
	const q = `UPDATE oncall_schedules SET
		name=$2, rotation=$3, participants=$4, timezone=$5, anchor=$6, updated_at=$7
		WHERE id=$1`
	tag, err := dbFrom(ctx, r.pool).Exec(ctx, q,
		s.ID().String(), s.Name(), s.Rotation().String(), s.Participants(),
		s.Timezone(), s.Anchor().UTC(), s.UpdatedAt())
	if err != nil {
		return fmt.Errorf("update schedule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrScheduleNotFound
	}
	return nil
}

// ByID đọc schedule theo id; ErrScheduleNotFound nếu không có.
func (r *ScheduleRepo) ByID(ctx context.Context, id domain.ScheduleID) (domain.Schedule, error) {
	const q = `SELECT ` + scheduleColumns + ` FROM oncall_schedules WHERE id = $1`
	s, err := scanSchedule(dbFrom(ctx, r.pool).QueryRow(ctx, q, id.String()))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Schedule{}, domain.ErrScheduleNotFound
	}
	return s, err
}

// List đọc schedule của workspace (cũ nhất trước — schedule chính lên đầu).
func (r *ScheduleRepo) List(ctx context.Context, workspaceID string) ([]domain.Schedule, error) {
	const q = `SELECT ` + scheduleColumns + ` FROM oncall_schedules
		WHERE workspace_id = $1 ORDER BY created_at ASC`
	rows, err := dbFrom(ctx, r.pool).Query(ctx, q, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("query schedules: %w", err)
	}
	defer rows.Close()

	var out []domain.Schedule
	for rows.Next() {
		s, err := scanSchedule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate schedules: %w", err)
	}
	return out, nil
}

func scanSchedule(row scanRow) (domain.Schedule, error) {
	var (
		id, workspaceID, name, rotation, timezone string
		participants                              []string
		anchor, createdAt, updatedAt              time.Time
	)
	if err := row.Scan(&id, &workspaceID, &name, &rotation, &participants, &timezone,
		&anchor, &createdAt, &updatedAt); err != nil {
		return domain.Schedule{}, err
	}
	return domain.ReconstructSchedule(domain.ReconstructScheduleInput{
		ID:           id,
		WorkspaceID:  workspaceID,
		Name:         name,
		Rotation:     rotation,
		Participants: participants,
		Timezone:     timezone,
		Anchor:       anchor.UTC(),
		CreatedAt:    createdAt.UTC(),
		UpdatedAt:    updatedAt.UTC(),
	}), nil
}

// OverrideRepo lưu trữ + đọc override on-call.
type OverrideRepo struct {
	pool *pgxpool.Pool
}

var (
	_ ports.OverrideRepository = (*OverrideRepo)(nil)
	_ ports.OverrideReader     = (*OverrideRepo)(nil)
)

// NewOverrideRepo tạo repo với pool.
func NewOverrideRepo(pool *pgxpool.Pool) *OverrideRepo { return &OverrideRepo{pool: pool} }

// Save chèn override mới.
func (r *OverrideRepo) Save(ctx context.Context, o domain.Override) error {
	const q = `INSERT INTO oncall_overrides
		(id, schedule_id, participant, start_at, end_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6)`
	_, err := dbFrom(ctx, r.pool).Exec(ctx, q,
		o.ID(), o.ScheduleID().String(), o.Participant(), o.StartAt(), o.EndAt(), o.CreatedAt())
	if err != nil {
		return fmt.Errorf("insert override: %w", err)
	}
	return nil
}

// ActiveForSchedule đọc override phủ thời điểm `at` của schedule.
func (r *OverrideRepo) ActiveForSchedule(ctx context.Context, scheduleID domain.ScheduleID, at time.Time) ([]domain.Override, error) {
	const q = `SELECT id, schedule_id, participant, start_at, end_at, created_at
		FROM oncall_overrides
		WHERE schedule_id = $1 AND start_at <= $2 AND end_at > $2
		ORDER BY start_at ASC`
	rows, err := dbFrom(ctx, r.pool).Query(ctx, q, scheduleID.String(), at)
	if err != nil {
		return nil, fmt.Errorf("query overrides: %w", err)
	}
	defer rows.Close()

	var out []domain.Override
	for rows.Next() {
		var (
			id, sid, participant      string
			startAt, endAt, createdAt time.Time
		)
		if err := rows.Scan(&id, &sid, &participant, &startAt, &endAt, &createdAt); err != nil {
			return nil, err
		}
		out = append(out, domain.ReconstructOverride(id, sid, participant, startAt.UTC(), endAt.UTC(), createdAt.UTC()))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate overrides: %w", err)
	}
	return out, nil
}

// EscalationPolicyRepo lưu trữ + đọc escalation policy (1/workspace).
type EscalationPolicyRepo struct {
	pool *pgxpool.Pool
}

var (
	_ ports.EscalationPolicyRepository = (*EscalationPolicyRepo)(nil)
	_ ports.EscalationPolicyReader     = (*EscalationPolicyRepo)(nil)
)

// NewEscalationPolicyRepo tạo repo với pool.
func NewEscalationPolicyRepo(pool *pgxpool.Pool) *EscalationPolicyRepo {
	return &EscalationPolicyRepo{pool: pool}
}

type levelJSON struct {
	Target         string `json:"target"`
	TimeoutSeconds int64  `json:"timeout_seconds"`
}

// Save upsert policy theo workspace (1 policy/workspace).
func (r *EscalationPolicyRepo) Save(ctx context.Context, p domain.EscalationPolicy) error {
	levels := make([]levelJSON, 0, len(p.Levels()))
	for _, lv := range p.Levels() {
		levels = append(levels, levelJSON{
			Target:         lv.Target().String(),
			TimeoutSeconds: int64(lv.Timeout() / time.Second),
		})
	}
	raw, err := json.Marshal(levels)
	if err != nil {
		return fmt.Errorf("marshal levels: %w", err)
	}
	const q = `INSERT INTO escalation_policies (id, workspace_id, name, team_lead, levels)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (workspace_id) DO UPDATE SET
			name = EXCLUDED.name, team_lead = EXCLUDED.team_lead, levels = EXCLUDED.levels`
	if _, err := dbFrom(ctx, r.pool).Exec(ctx, q,
		p.ID(), p.WorkspaceID(), p.Name(), p.TeamLead(), raw); err != nil {
		return fmt.Errorf("upsert escalation policy: %w", err)
	}
	return nil
}

// ByWorkspace đọc policy của workspace; ErrEscalationPolicyNotFound nếu chưa có.
func (r *EscalationPolicyRepo) ByWorkspace(ctx context.Context, workspaceID string) (domain.EscalationPolicy, error) {
	const q = `SELECT id, workspace_id, name, team_lead, levels
		FROM escalation_policies WHERE workspace_id = $1`
	var (
		id, ws, name, teamLead string
		raw                    []byte
	)
	err := dbFrom(ctx, r.pool).QueryRow(ctx, q, workspaceID).Scan(&id, &ws, &name, &teamLead, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.EscalationPolicy{}, domain.ErrEscalationPolicyNotFound
	}
	if err != nil {
		return domain.EscalationPolicy{}, fmt.Errorf("query escalation policy: %w", err)
	}
	var levels []levelJSON
	if err := json.Unmarshal(raw, &levels); err != nil {
		return domain.EscalationPolicy{}, fmt.Errorf("unmarshal levels: %w", err)
	}
	domainLevels := make([]domain.EscalationLevel, 0, len(levels))
	for _, lv := range levels {
		domainLevels = append(domainLevels,
			domain.ReconstructEscalationLevel(lv.Target, time.Duration(lv.TimeoutSeconds)*time.Second))
	}
	return domain.ReconstructEscalationPolicy(id, ws, name, teamLead, domainLevels), nil
}

// EscalationStateRepo theo dõi bậc escalation đã thông báo theo incident.
type EscalationStateRepo struct {
	pool *pgxpool.Pool
}

var (
	_ ports.EscalationStateReader     = (*EscalationStateRepo)(nil)
	_ ports.EscalationStateRepository = (*EscalationStateRepo)(nil)
)

// NewEscalationStateRepo tạo repo với pool.
func NewEscalationStateRepo(pool *pgxpool.Pool) *EscalationStateRepo {
	return &EscalationStateRepo{pool: pool}
}

// HighestNotified trả bậc cao nhất đã thông báo của incident (-1 nếu chưa có).
func (r *EscalationStateRepo) HighestNotified(ctx context.Context, incidentID domain.IncidentID) (int, error) {
	const q = `SELECT COALESCE(MAX(level), -1) FROM incident_escalations WHERE incident_id = $1`
	var highest int
	if err := dbFrom(ctx, r.pool).QueryRow(ctx, q, incidentID.String()).Scan(&highest); err != nil {
		return -1, fmt.Errorf("query escalation state: %w", err)
	}
	return highest, nil
}

// Record ghi nhận một bậc đã thông báo (idempotent theo (incident_id, level)).
func (r *EscalationStateRepo) Record(ctx context.Context, incidentID domain.IncidentID, level int, recipient string, at time.Time) error {
	const q = `INSERT INTO incident_escalations (incident_id, level, recipient, notified_at)
		VALUES ($1,$2,$3,$4) ON CONFLICT (incident_id, level) DO NOTHING`
	if _, err := dbFrom(ctx, r.pool).Exec(ctx, q, incidentID.String(), level, recipient, at.UTC()); err != nil {
		return fmt.Errorf("record escalation: %w", err)
	}
	return nil
}
