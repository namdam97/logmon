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

// fakeInstanceRepo ghi nhận lời gọi upsert/resolve.
type fakeInstanceRepo struct {
	upserted   []domain.AlertInstance
	resolved   []string // fingerprint đã resolve
	upsertErr  error
	resolveErr error
}

func (r *fakeInstanceRepo) UpsertFiring(_ context.Context, inst domain.AlertInstance) error {
	if r.upsertErr != nil {
		return r.upsertErr
	}
	r.upserted = append(r.upserted, inst)
	return nil
}

func (r *fakeInstanceRepo) Resolve(_ context.Context, _, fingerprint string, _ time.Time) error {
	if r.resolveErr != nil {
		return r.resolveErr
	}
	r.resolved = append(r.resolved, fingerprint)
	return nil
}

const wsDefault = "00000000-0000-0000-0000-000000000001"

func newIngestHandler(repo *fakeInstanceRepo) *command.IngestWebhookHandler {
	return command.NewIngestWebhookHandler(
		fakeTx{}, repo,
		fixedID{id: "11111111-1111-1111-1111-111111111111"}, fixedClock{},
	)
}

func TestIngestWebhook_FiringAndResolved(t *testing.T) {
	repo := &fakeInstanceRepo{}
	h := newIngestHandler(repo)

	res, err := h.Handle(context.Background(), command.IngestWebhookInput{
		WorkspaceID: wsDefault,
		Alerts: []command.WebhookAlert{
			{
				Status:      "firing",
				Fingerprint: "aaaa1111",
				StartsAt:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				Labels:      map[string]string{"alertname": "HighErrorRate", "severity": "critical"},
			},
			{
				Status:      "resolved",
				Fingerprint: "bbbb2222",
				StartsAt:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				EndsAt:      time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC),
				Labels:      map[string]string{"alertname": "DiskFull"},
			},
		},
	})

	require.NoError(t, err)
	require.Equal(t, 1, res.Firing)
	require.Equal(t, 1, res.Resolved)
	require.Len(t, repo.upserted, 1)
	require.Equal(t, "aaaa1111", repo.upserted[0].Fingerprint().String())
	require.Equal(t, domain.InstanceFiring, repo.upserted[0].Status())
	require.Equal(t, []string{"bbbb2222"}, repo.resolved)
}

func TestIngestWebhook_EmptyFingerprintRejected(t *testing.T) {
	repo := &fakeInstanceRepo{}
	h := newIngestHandler(repo)

	_, err := h.Handle(context.Background(), command.IngestWebhookInput{
		WorkspaceID: wsDefault,
		Alerts: []command.WebhookAlert{
			{Status: "firing", Fingerprint: "", StartsAt: time.Now()},
		},
	})

	var ve *domain.ValidationError
	require.True(t, errors.As(err, &ve))
	require.Empty(t, repo.upserted)
}

func TestIngestWebhook_UnknownStatusRejected(t *testing.T) {
	repo := &fakeInstanceRepo{}
	h := newIngestHandler(repo)

	_, err := h.Handle(context.Background(), command.IngestWebhookInput{
		WorkspaceID: wsDefault,
		Alerts: []command.WebhookAlert{
			{Status: "pending", Fingerprint: "cccc3333", StartsAt: time.Now()},
		},
	})

	var ve *domain.ValidationError
	require.True(t, errors.As(err, &ve))
}

func TestIngestWebhook_ResolvedWithoutEndsAtUsesClock(t *testing.T) {
	repo := &fakeInstanceRepo{}
	h := newIngestHandler(repo)

	res, err := h.Handle(context.Background(), command.IngestWebhookInput{
		WorkspaceID: wsDefault,
		Alerts: []command.WebhookAlert{
			{Status: "resolved", Fingerprint: "dddd4444", StartsAt: time.Now()},
		},
	})

	require.NoError(t, err)
	require.Equal(t, 1, res.Resolved)
	require.Equal(t, []string{"dddd4444"}, repo.resolved)
}
