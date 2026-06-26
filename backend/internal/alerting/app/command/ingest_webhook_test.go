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

// fakeInstanceRepo ghi nhận lời gọi upsert/resolve/acknowledge và phục vụ ByID
// (implement cả AlertInstanceRepository lẫn AlertInstanceReader như adapter thật).
type fakeInstanceRepo struct {
	upserted   []domain.AlertInstance
	resolved   []string // fingerprint đã resolve
	acked      []domain.AlertInstance
	stored     map[string]domain.AlertInstance // id → instance, cho ByID
	upsertErr  error
	resolveErr error
	ackErr     error
	byIDErr    error
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

func (r *fakeInstanceRepo) Acknowledge(_ context.Context, inst domain.AlertInstance) error {
	if r.ackErr != nil {
		return r.ackErr
	}
	r.acked = append(r.acked, inst)
	return nil
}

func (r *fakeInstanceRepo) ByID(_ context.Context, _, id string) (domain.AlertInstance, error) {
	if r.byIDErr != nil {
		return domain.AlertInstance{}, r.byIDErr
	}
	inst, ok := r.stored[id]
	if !ok {
		return domain.AlertInstance{}, domain.ErrInstanceNotFound
	}
	return inst, nil
}

func (r *fakeInstanceRepo) ListActive(_ context.Context, _ string) ([]domain.AlertInstance, error) {
	return nil, nil
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
