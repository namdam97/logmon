package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/alerting/domain"
)

func mustFingerprint(t *testing.T, raw string) domain.Fingerprint {
	t.Helper()
	fp, err := domain.NewFingerprint(raw)
	require.NoError(t, err)
	return fp
}

func TestNewFingerprint(t *testing.T) {
	tests := []struct {
		name    string
		give    string
		wantErr bool
	}{
		{name: "valid", give: "a1b2c3d4e5f60718", wantErr: false},
		{name: "empty", give: "", wantErr: true},
		{name: "too long", give: strings.Repeat("x", 65), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp, err := domain.NewFingerprint(tt.give)
			if tt.wantErr {
				var ve *domain.ValidationError
				require.True(t, errors.As(err, &ve))
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.give, fp.String())
		})
	}
}

func validFiringInput(t *testing.T) domain.NewFiringInstanceInput {
	t.Helper()
	return domain.NewFiringInstanceInput{
		ID:          "11111111-1111-1111-1111-111111111111",
		WorkspaceID: "00000000-0000-0000-0000-000000000001",
		Fingerprint: mustFingerprint(t, "a1b2c3d4e5f60718"),
		FiredAt:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Labels:      map[string]string{"alertname": "HighErrorRate", "severity": "critical"},
	}
}

func TestNewFiringInstance_Success(t *testing.T) {
	inst, err := domain.NewFiringInstance(validFiringInput(t))

	require.NoError(t, err)
	require.Equal(t, domain.InstanceFiring, inst.Status())
	require.Equal(t, "a1b2c3d4e5f60718", inst.Fingerprint().String())
	require.Equal(t, "HighErrorRate", inst.Labels()["alertname"])
	require.True(t, inst.ResolvedAt().IsZero())
}

func TestNewFiringInstance_Invalid(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(in *domain.NewFiringInstanceInput)
	}{
		{name: "empty id", mutate: func(in *domain.NewFiringInstanceInput) { in.ID = "" }},
		{name: "empty workspace", mutate: func(in *domain.NewFiringInstanceInput) { in.WorkspaceID = "" }},
		{name: "zero firedAt", mutate: func(in *domain.NewFiringInstanceInput) { in.FiredAt = time.Time{} }},
		{name: "zero fingerprint", mutate: func(in *domain.NewFiringInstanceInput) {
			in.Fingerprint = domain.Fingerprint{}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validFiringInput(t)
			tt.mutate(&in)

			_, err := domain.NewFiringInstance(in)

			var ve *domain.ValidationError
			require.True(t, errors.As(err, &ve))
		})
	}
}

func TestAlertInstance_ResolveIsImmutable(t *testing.T) {
	inst, err := domain.NewFiringInstance(validFiringInput(t))
	require.NoError(t, err)
	at := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)

	resolved := inst.Resolve(at)

	require.Equal(t, domain.InstanceResolved, resolved.Status())
	require.Equal(t, at, resolved.ResolvedAt())
	// Bản gốc không đổi (immutability).
	require.Equal(t, domain.InstanceFiring, inst.Status())
	require.True(t, inst.ResolvedAt().IsZero())
}

func TestAlertInstance_AcknowledgeIsImmutable(t *testing.T) {
	inst, err := domain.NewFiringInstance(validFiringInput(t))
	require.NoError(t, err)
	at := time.Date(2026, 1, 1, 0, 30, 0, 0, time.UTC)
	const by = "22222222-2222-2222-2222-222222222222"

	acked, err := inst.Acknowledge(by, at)

	require.NoError(t, err)
	require.Equal(t, domain.InstanceAcknowledged, acked.Status())
	require.Equal(t, at, acked.AcknowledgedAt())
	require.Equal(t, by, acked.AcknowledgedBy())
	// Bản gốc không đổi (immutability).
	require.Equal(t, domain.InstanceFiring, inst.Status())
	require.True(t, inst.AcknowledgedAt().IsZero())
	require.Empty(t, inst.AcknowledgedBy())
}

func TestAlertInstance_AcknowledgeRequiresFiring(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 30, 0, 0, time.UTC)
	const by = "22222222-2222-2222-2222-222222222222"

	tests := []struct {
		name    string
		prepare func(t *testing.T) domain.AlertInstance
	}{
		{
			name: "already resolved",
			prepare: func(t *testing.T) domain.AlertInstance {
				inst, err := domain.NewFiringInstance(validFiringInput(t))
				require.NoError(t, err)
				return inst.Resolve(at)
			},
		},
		{
			name: "already acknowledged",
			prepare: func(t *testing.T) domain.AlertInstance {
				inst, err := domain.NewFiringInstance(validFiringInput(t))
				require.NoError(t, err)
				acked, err := inst.Acknowledge(by, at)
				require.NoError(t, err)
				return acked
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.prepare(t).Acknowledge(by, at)
			require.ErrorIs(t, err, domain.ErrInstanceNotAcknowledgeable)
		})
	}
}

func TestAlertInstance_AcknowledgeRequiresActor(t *testing.T) {
	inst, err := domain.NewFiringInstance(validFiringInput(t))
	require.NoError(t, err)

	_, err = inst.Acknowledge("", time.Date(2026, 1, 1, 0, 30, 0, 0, time.UTC))

	var ve *domain.ValidationError
	require.True(t, errors.As(err, &ve))
}

func TestAlertInstance_LabelsCopiedOnRead(t *testing.T) {
	inst, err := domain.NewFiringInstance(validFiringInput(t))
	require.NoError(t, err)

	got := inst.Labels()
	got["alertname"] = "tampered"

	require.Equal(t, "HighErrorRate", inst.Labels()["alertname"], "label nội bộ không bị mutate qua reference")
}
