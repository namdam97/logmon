package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/namdam97/logmon/backend/internal/shared/outbox"
	"github.com/namdam97/logmon/backend/internal/slo/ports"
)

// EventPublisher ghi domain event vào outbox dùng outbox.Store, trong tx của ctx
// (cùng TX với SLO INSERT → transactional outbox).
type EventPublisher struct {
	pool  *pgxpool.Pool
	store *outbox.Store
}

var _ ports.EventPublisher = (*EventPublisher)(nil)

// NewEventPublisher tạo publisher với pool + outbox store.
func NewEventPublisher(pool *pgxpool.Pool, store *outbox.Store) *EventPublisher {
	return &EventPublisher{pool: pool, store: store}
}

// Publish marshal payload sang JSON và ghi outbox event qua tx của ctx.
func (p *EventPublisher) Publish(ctx context.Context, aggregateType, aggregateID, eventType string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal event payload: %w", err)
	}
	return p.store.Save(ctx, dbFrom(ctx, p.pool), outbox.Event{
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		EventType:     eventType,
		Payload:       data,
	})
}
