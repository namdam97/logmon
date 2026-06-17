package command_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/alerting/app/command"
	"github.com/namdam97/logmon/backend/internal/alerting/domain"
)

// --- test doubles ---

type fakeTx struct{}

func (fakeTx) WithinTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx) // không tx thật trong unit test
}

type fakeRepo struct {
	names   map[string]bool // "ws|name" → tồn tại
	saved   []domain.AlertRule
	saveErr error
}

func newFakeRepo() *fakeRepo { return &fakeRepo{names: map[string]bool{}} }

func (r *fakeRepo) ExistsByName(_ context.Context, ws, name string) (bool, error) {
	return r.names[ws+"|"+name], nil
}

func (r *fakeRepo) Save(_ context.Context, rule domain.AlertRule) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	r.saved = append(r.saved, rule)
	return nil
}

type fakePublisher struct {
	events []string // eventType đã publish
}

func (p *fakePublisher) Publish(_ context.Context, _, _, eventType string, _ any) error {
	p.events = append(p.events, eventType)
	return nil
}

type fakeValidator struct{ err error }

func (v fakeValidator) ValidateExpression(string) error { return v.err }

type fixedID struct{ id string }

func (g fixedID) NewID() string { return g.id }

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }

func newHandler(repo *fakeRepo, pub *fakePublisher, validator fakeValidator) *command.CreateRuleHandler {
	return command.NewCreateRuleHandler(
		fakeTx{}, repo, pub, validator,
		fixedID{id: "11111111-1111-1111-1111-111111111111"}, fixedClock{},
	)
}

func validInput() command.CreateRuleInput {
	return command.CreateRuleInput{
		WorkspaceID: "ws-default",
		Name:        "HighErrorRate",
		Expression:  `rate(logmon_http_requests_total{status=~"5.."}[5m]) > 0.05`,
		Service:     "logmon-api",
		Severity:    "critical",
		ForDuration: 2 * time.Minute,
		Annotations: map[string]string{
			domain.AnnotationSummary:    "High 5xx",
			domain.AnnotationRunbookURL: "https://wiki/runbooks/HighErrorRate",
		},
	}
}

func TestCreateRule_Success(t *testing.T) {
	repo, pub := newFakeRepo(), &fakePublisher{}
	h := newHandler(repo, pub, fakeValidator{})

	rule, err := h.Handle(context.Background(), validInput())

	require.NoError(t, err)
	require.Equal(t, "HighErrorRate", rule.Name())
	require.Equal(t, domain.SyncPending, rule.SyncStatus())
	require.Len(t, repo.saved, 1, "rule được persist")
	require.Equal(t, []string{domain.EventAlertRuleCreated}, pub.events, "phát đúng 1 event")
}

func TestCreateRule_InvalidPromQL(t *testing.T) {
	repo, pub := newFakeRepo(), &fakePublisher{}
	h := newHandler(repo, pub, fakeValidator{err: errors.New("parse error")})

	_, err := h.Handle(context.Background(), validInput())

	var ve *domain.ValidationError
	require.True(t, errors.As(err, &ve))
	require.Equal(t, "expression", ve.Field)
	require.Empty(t, repo.saved, "không persist khi PromQL sai")
	require.Empty(t, pub.events, "không phát event khi PromQL sai")
}

func TestCreateRule_DuplicateName(t *testing.T) {
	repo, pub := newFakeRepo(), &fakePublisher{}
	repo.names["ws-default|HighErrorRate"] = true
	h := newHandler(repo, pub, fakeValidator{})

	_, err := h.Handle(context.Background(), validInput())

	require.ErrorIs(t, err, domain.ErrRuleNameTaken)
	require.Empty(t, repo.saved)
	require.Empty(t, pub.events)
}

func TestCreateRule_DomainInvariants(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(in *command.CreateRuleInput)
	}{
		{name: "invalid severity", mutate: func(in *command.CreateRuleInput) { in.Severity = "fatal" }},
		{name: "for below critical min", mutate: func(in *command.CreateRuleInput) { in.ForDuration = 10 * time.Second }},
		{name: "missing runbook", mutate: func(in *command.CreateRuleInput) {
			in.Annotations = map[string]string{domain.AnnotationSummary: "s"}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, pub := newFakeRepo(), &fakePublisher{}
			h := newHandler(repo, pub, fakeValidator{})
			in := validInput()
			tt.mutate(&in)

			_, err := h.Handle(context.Background(), in)

			var ve *domain.ValidationError
			require.True(t, errors.As(err, &ve))
			require.Empty(t, repo.saved)
			require.Empty(t, pub.events)
		})
	}
}
