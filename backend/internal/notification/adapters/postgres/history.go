package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/namdam97/logmon/backend/internal/notification/domain"
	"github.com/namdam97/logmon/backend/internal/notification/ports"
)

// _maxHistoryLimit chặn truy vấn không giới hạn (mặc định khi limit<=0).
const _maxHistoryLimit = 200

// HistoryRepository ghi + đọc lịch sử gửi.
type HistoryRepository struct {
	pool *pgxpool.Pool
}

var (
	_ ports.HistoryWriter = (*HistoryRepository)(nil)
	_ ports.HistoryReader = (*HistoryRepository)(nil)
)

// NewHistoryRepository tạo repository với pool.
func NewHistoryRepository(pool *pgxpool.Pool) *HistoryRepository {
	return &HistoryRepository{pool: pool}
}

// Save ghi một bản ghi kết quả gửi.
func (r *HistoryRepository) Save(ctx context.Context, h domain.HistoryEntry) error {
	const q = `INSERT INTO notification_history
		(workspace_id, channel_id, event_type, event_ref, status, response_code, error_message, sent_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	_, err := dbFrom(ctx, r.pool).Exec(ctx, q,
		h.WorkspaceID, h.ChannelID, h.EventType, h.EventRef, string(h.Status),
		nullableCode(h.ResponseCode), nullableText(h.ErrorMessage), h.SentAt)
	if err != nil {
		return fmt.Errorf("insert history: %w", err)
	}
	return nil
}

// List đọc lịch sử mới nhất của workspace (giới hạn limit, cap _maxHistoryLimit).
func (r *HistoryRepository) List(ctx context.Context, workspaceID string, limit int) ([]domain.HistoryEntry, error) {
	if limit <= 0 || limit > _maxHistoryLimit {
		limit = _maxHistoryLimit
	}
	const q = `SELECT workspace_id, channel_id, event_type, event_ref, status,
		response_code, error_message, sent_at
		FROM notification_history WHERE workspace_id = $1 ORDER BY sent_at DESC LIMIT $2`
	rows, err := dbFrom(ctx, r.pool).Query(ctx, q, workspaceID, limit)
	if err != nil {
		return nil, fmt.Errorf("query history: %w", err)
	}
	defer rows.Close()

	var entries []domain.HistoryEntry
	for rows.Next() {
		var (
			wsID, chID, eventType, eventRef, status string
			code                                    *int
			errMsg                                  *string
			sentAt                                  time.Time
		)
		if err := rows.Scan(&wsID, &chID, &eventType, &eventRef, &status, &code, &errMsg, &sentAt); err != nil {
			return nil, fmt.Errorf("scan history: %w", err)
		}
		entries = append(entries, domain.HistoryEntry{
			WorkspaceID:  wsID,
			ChannelID:    chID,
			EventType:    eventType,
			EventRef:     eventRef,
			Status:       domain.DeliveryStatus(status),
			ResponseCode: deref(code),
			ErrorMessage: derefStr(errMsg),
			SentAt:       sentAt.UTC(),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate history: %w", err)
	}
	return entries, nil
}

func nullableCode(c int) *int {
	if c == 0 {
		return nil
	}
	return &c
}

func nullableText(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
