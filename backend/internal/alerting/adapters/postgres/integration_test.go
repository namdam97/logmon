//go:build integration

// Integration test cho persistence alerting — cần Postgres thật (DATABASE_URL)
// đã áp migrations. Chạy: make test-integration.
package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/alerting/adapters/postgres"
	"github.com/namdam97/logmon/backend/internal/alerting/adapters/promql"
	"github.com/namdam97/logmon/backend/internal/alerting/app/command"
	"github.com/namdam97/logmon/backend/internal/alerting/domain"
	"github.com/namdam97/logmon/backend/internal/shared/outbox"
)

type uuidGen struct{}

func (uuidGen) NewID() string { return uuid.NewString() }

type sysClock struct{}

func (sysClock) Now() time.Time { return time.Now().UTC() }

func newPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dburl := os.Getenv("DATABASE_URL")
	if dburl == "" {
		t.Skip("DATABASE_URL chưa set — bỏ qua integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dburl)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	_, err = pool.Exec(context.Background(), "TRUNCATE alert_rules, outbox_events RESTART IDENTITY")
	require.NoError(t, err)
	return pool
}

func newHandler(pool *pgxpool.Pool) *command.CreateRuleHandler {
	return command.NewCreateRuleHandler(
		postgres.NewTxManager(pool),
		postgres.NewRuleRepository(pool),
		postgres.NewEventPublisher(pool, outbox.NewStore(pool)),
		promql.NewValidator(),
		uuidGen{},
		sysClock{},
	)
}

func validInput() command.CreateRuleInput {
	return command.CreateRuleInput{
		WorkspaceID: uuid.NewString(),
		Name:        "HighErrorRate",
		Expression:  `rate(logmon_http_requests_total{status=~"5.."}[5m]) > 0.05`,
		Service:     "logmon-api",
		Severity:    "critical",
		ForDuration: 2 * time.Minute,
		Labels:      map[string]string{"team": "backend"},
		Annotations: map[string]string{
			domain.AnnotationSummary:    "High 5xx",
			domain.AnnotationRunbookURL: "https://wiki/runbooks/HighErrorRate",
		},
	}
}

func count(t *testing.T, pool *pgxpool.Pool, q string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(context.Background(), q, args...).Scan(&n))
	return n
}

func TestCreateRule_PersistsRuleAndOutboxInSameTx(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	h := newHandler(pool)
	in := validInput()

	rule, err := h.Handle(ctx, in)
	require.NoError(t, err)

	// Rule + outbox event được ghi trong cùng TX.
	require.Equal(t, 1, count(t, pool, "SELECT count(*) FROM alert_rules WHERE name = $1", in.Name))
	require.Equal(t, 1, count(t, pool,
		"SELECT count(*) FROM outbox_events WHERE event_type = $1 AND aggregate_type = $2 AND aggregate_id = $3",
		domain.EventAlertRuleCreated, domain.AggregateType, rule.ID().String()))

	// Đọc lại qua reader: state đúng (enabled, pending, for-duration, labels).
	repo := postgres.NewRuleRepository(pool)
	got, err := repo.ByID(ctx, rule.ID())
	require.NoError(t, err)
	require.Equal(t, in.Name, got.Name())
	require.Equal(t, domain.SeverityCritical.String(), got.Severity().String())
	require.True(t, got.IsEnabled())
	require.Equal(t, domain.SyncPending, got.SyncStatus())
	require.Equal(t, 2*time.Minute, got.ForDuration())
	require.Equal(t, "backend", got.Labels()["team"])
}

func TestCreateRule_DuplicateNameRollsBack(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	h := newHandler(pool)
	in := validInput()

	_, err := h.Handle(ctx, in)
	require.NoError(t, err)

	// Tạo lại cùng tên trong cùng workspace → ErrRuleNameTaken, không ghi thêm.
	dup := in
	_, err = h.Handle(ctx, dup)
	require.ErrorIs(t, err, domain.ErrRuleNameTaken)
	require.Equal(t, 1, count(t, pool, "SELECT count(*) FROM alert_rules"))
	require.Equal(t, 1, count(t, pool, "SELECT count(*) FROM outbox_events"))
}
