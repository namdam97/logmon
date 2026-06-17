package outbox

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Execer là phần giao của pgx.Tx và pgxpool.Pool mà Save cần — cho phép ghi
// event trong TX của caller (transactional outbox).
type Execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Store là adapter Postgres cho outbox (Save + relay Processor).
type Store struct {
	pool       *pgxpool.Pool
	maxRetries int
}

var _ Processor = (*Store)(nil)

// StoreOption cấu hình Store.
type StoreOption func(*Store)

// WithMaxRetries đặt số lần retry tối đa trước khi đánh failed.
func WithMaxRetries(n int) StoreOption { return func(s *Store) { s.maxRetries = n } }

// NewStore tạo Store với pool.
func NewStore(pool *pgxpool.Pool, opts ...StoreOption) *Store {
	s := &Store{pool: pool, maxRetries: DefaultMaxRetries}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Save ghi event vào outbox trong TX của caller (cùng TX với state change).
func (s *Store) Save(ctx context.Context, tx Execer, e Event) error {
	const q = `INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload)
	           VALUES ($1, $2, $3, $4)`
	if _, err := tx.Exec(ctx, q, e.AggregateType, e.AggregateID, e.EventType, string(e.Payload)); err != nil {
		return fmt.Errorf("insert outbox: %w", err)
	}
	return nil
}

// OldestPendingAge trả tuổi event pending cũ nhất (cho metric lag).
func (s *Store) OldestPendingAge(ctx context.Context) (time.Duration, bool, error) {
	const q = `SELECT created_at FROM outbox_events WHERE status = 'pending' ORDER BY id LIMIT 1`
	var createdAt time.Time
	switch err := s.pool.QueryRow(ctx, q).Scan(&createdAt); {
	case err == nil:
		return time.Since(createdAt), true, nil
	case errors.Is(err, pgx.ErrNoRows):
		return 0, false, nil
	default:
		return 0, false, fmt.Errorf("oldest pending: %w", err)
	}
}

// ProcessBatch claim tối đa limit event pending bằng FOR UPDATE SKIP LOCKED,
// dispatch từng event, rồi mark published/failed — tất cả trong một TX.
func (s *Store) ProcessBatch(ctx context.Context, limit int, dispatch Handler) (int, int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op sau Commit

	events, err := claimPending(ctx, tx, limit)
	if err != nil {
		return 0, 0, err
	}
	if len(events) == 0 {
		return 0, 0, nil
	}

	var published, failed, retry []int64
	for _, e := range events {
		if derr := dispatch(ctx, e); derr == nil {
			published = append(published, e.ID)
		} else if e.RetryCount+1 >= s.maxRetries {
			failed = append(failed, e.ID)
		} else {
			retry = append(retry, e.ID)
		}
	}

	if err := applyOutcomes(ctx, tx, published, failed, retry); err != nil {
		return 0, 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, 0, fmt.Errorf("commit: %w", err)
	}
	return len(events), len(failed), nil
}

func claimPending(ctx context.Context, tx pgx.Tx, limit int) ([]Event, error) {
	const q = `SELECT id, aggregate_type, aggregate_id, event_type, payload, retry_count, created_at
	           FROM outbox_events
	           WHERE status = 'pending'
	           ORDER BY id
	           LIMIT $1
	           FOR UPDATE SKIP LOCKED`
	rows, err := tx.Query(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("claim pending: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.AggregateType, &e.AggregateID, &e.EventType,
			&e.Payload, &e.RetryCount, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan outbox: %w", err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate outbox: %w", err)
	}
	return events, nil
}

func applyOutcomes(ctx context.Context, tx pgx.Tx, published, failed, retry []int64) error {
	if len(published) > 0 {
		const q = `UPDATE outbox_events SET status = 'published', published_at = now() WHERE id = ANY($1)`
		if _, err := tx.Exec(ctx, q, published); err != nil {
			return fmt.Errorf("mark published: %w", err)
		}
	}
	if len(failed) > 0 {
		const q = `UPDATE outbox_events SET status = 'failed', retry_count = retry_count + 1 WHERE id = ANY($1)`
		if _, err := tx.Exec(ctx, q, failed); err != nil {
			return fmt.Errorf("mark failed: %w", err)
		}
	}
	if len(retry) > 0 {
		const q = `UPDATE outbox_events SET retry_count = retry_count + 1 WHERE id = ANY($1)`
		if _, err := tx.Exec(ctx, q, retry); err != nil {
			return fmt.Errorf("mark retry: %w", err)
		}
	}
	return nil
}
