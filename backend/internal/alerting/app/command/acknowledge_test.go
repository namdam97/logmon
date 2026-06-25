package command_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/alerting/app/command"
	"github.com/namdam97/logmon/backend/internal/alerting/domain"
)

const (
	ackInstanceID = "11111111-1111-1111-1111-111111111111"
	ackActor      = "22222222-2222-2222-2222-222222222222"
)

func firingInstanceForAck(t *testing.T) domain.AlertInstance {
	t.Helper()
	fp, err := domain.NewFingerprint("aaaa1111")
	require.NoError(t, err)
	inst, err := domain.NewFiringInstance(domain.NewFiringInstanceInput{
		ID:          ackInstanceID,
		WorkspaceID: wsDefault,
		Fingerprint: fp,
		FiredAt:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Labels:      map[string]string{"alertname": "HighErrorRate"},
	})
	require.NoError(t, err)
	return inst
}

func newAckHandler(repo *fakeInstanceRepo) *command.AcknowledgeHandler {
	return command.NewAcknowledgeHandler(fakeTx{}, repo, repo, fixedClock{})
}

func TestAcknowledge_Success(t *testing.T) {
	repo := &fakeInstanceRepo{stored: map[string]domain.AlertInstance{
		ackInstanceID: firingInstanceForAck(t),
	}}
	h := newAckHandler(repo)

	got, err := h.Handle(context.Background(), command.AcknowledgeInput{
		WorkspaceID: wsDefault,
		InstanceID:  ackInstanceID,
		AckedBy:     ackActor,
	})

	require.NoError(t, err)
	require.Equal(t, domain.InstanceAcknowledged, got.Status())
	require.Equal(t, ackActor, got.AcknowledgedBy())
	require.Equal(t, fixedClock{}.Now(), got.AcknowledgedAt())
	require.Len(t, repo.acked, 1)
	require.Equal(t, domain.InstanceAcknowledged, repo.acked[0].Status())
}

func TestAcknowledge_NotFound(t *testing.T) {
	repo := &fakeInstanceRepo{stored: map[string]domain.AlertInstance{}}
	h := newAckHandler(repo)

	_, err := h.Handle(context.Background(), command.AcknowledgeInput{
		WorkspaceID: wsDefault,
		InstanceID:  "does-not-exist",
		AckedBy:     ackActor,
	})

	require.ErrorIs(t, err, domain.ErrInstanceNotFound)
	require.Empty(t, repo.acked)
}

func TestAcknowledge_NotFiringRejected(t *testing.T) {
	resolved := firingInstanceForAck(t).Resolve(time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC))
	repo := &fakeInstanceRepo{stored: map[string]domain.AlertInstance{
		ackInstanceID: resolved,
	}}
	h := newAckHandler(repo)

	_, err := h.Handle(context.Background(), command.AcknowledgeInput{
		WorkspaceID: wsDefault,
		InstanceID:  ackInstanceID,
		AckedBy:     ackActor,
	})

	require.ErrorIs(t, err, domain.ErrInstanceNotAcknowledgeable)
	require.Empty(t, repo.acked)
}
