package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/slo/domain"
)

func giveValidInput() domain.NewSLOInput {
	return domain.NewSLOInput{
		ID:          mustID(),
		WorkspaceID: "ws-1",
		Name:        "checkout availability",
		Service:     "checkout",
		SLIType:     domain.SLIAvailability,
		Target:      0.999,
		WindowDays:  28,
		CreatedAt:   time.Date(2026, 6, 27, 0, 0, 0, 0, time.UTC),
	}
}

func mustID() domain.SLOID {
	id, err := domain.NewSLOID("slo-1")
	if err != nil {
		panic(err)
	}
	return id
}

func TestNewSLOAvailability(t *testing.T) {
	give := giveValidInput()

	got, err := domain.NewSLO(give)

	require.NoError(t, err)
	require.Equal(t, "checkout availability", got.Name())
	require.Equal(t, "checkout", got.Service())
	require.Equal(t, domain.SLIAvailability, got.SLIType())
	require.Equal(t, 0.999, got.Target())
	require.Equal(t, 28, got.WindowDays())
	require.Equal(t, domain.SyncPending, got.SyncStatus())
	require.InDelta(t, 0.001, got.ErrorBudget(), 1e-9)
	require.Equal(t, 0, got.LatencyThresholdMs())
}

func TestNewSLOLatencyRequiresThreshold(t *testing.T) {
	give := giveValidInput()
	give.SLIType = domain.SLILatency
	give.LatencyThresholdMs = 0

	_, err := domain.NewSLO(give)

	requireValidation(t, err, "latencyThresholdMs")
}

func TestNewSLOLatencyValid(t *testing.T) {
	give := giveValidInput()
	give.SLIType = domain.SLILatency
	give.LatencyThresholdMs = 300

	got, err := domain.NewSLO(give)

	require.NoError(t, err)
	require.True(t, got.SLIType().IsLatency())
	require.Equal(t, 300, got.LatencyThresholdMs())
}

func TestNewSLOAvailabilityRejectsThreshold(t *testing.T) {
	give := giveValidInput()
	give.LatencyThresholdMs = 300 // availability không được set threshold

	_, err := domain.NewSLO(give)

	requireValidation(t, err, "latencyThresholdMs")
}

func TestNewSLOWindowDefaultsTo28(t *testing.T) {
	give := giveValidInput()
	give.WindowDays = 0

	got, err := domain.NewSLO(give)

	require.NoError(t, err)
	require.Equal(t, domain.DefaultWindowDays, got.WindowDays())
}

func TestNewSLOValidation(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(in *domain.NewSLOInput)
		wantField string
	}{
		{"empty name", func(in *domain.NewSLOInput) { in.Name = "  " }, "name"},
		{"empty service", func(in *domain.NewSLOInput) { in.Service = "" }, "service"},
		{"target zero", func(in *domain.NewSLOInput) { in.Target = 0 }, "target"},
		{"target one", func(in *domain.NewSLOInput) { in.Target = 1 }, "target"},
		{"target above one", func(in *domain.NewSLOInput) { in.Target = 1.5 }, "target"},
		{"window too large", func(in *domain.NewSLOInput) { in.WindowDays = 365 }, "windowDays"},
		{"window negative", func(in *domain.NewSLOInput) { in.WindowDays = -1 }, "windowDays"},
		{"service injection", func(in *domain.NewSLOInput) { in.Service = `checkout"} or up{` }, "service"},
		{"name injection", func(in *domain.NewSLOInput) { in.Name = `bad"name` }, "name"},
		{"empty workspace", func(in *domain.NewSLOInput) { in.WorkspaceID = "" }, "workspaceId"},
		{"zero createdAt", func(in *domain.NewSLOInput) { in.CreatedAt = time.Time{} }, "createdAt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			give := giveValidInput()
			tt.mutate(&give)

			_, err := domain.NewSLO(give)

			requireValidation(t, err, tt.wantField)
		})
	}
}

func TestSLIType(t *testing.T) {
	_, err := domain.NewSLIType("throughput")
	requireValidation(t, err, "sliType")

	got, err := domain.NewSLIType("latency")
	require.NoError(t, err)
	require.Equal(t, domain.SLILatency, got)
}

func TestSLOUpdateResetsSync(t *testing.T) {
	s, err := domain.NewSLO(giveValidInput())
	require.NoError(t, err)
	synced := s.MarkSynced(time.Date(2026, 6, 27, 1, 0, 0, 0, time.UTC))
	require.Equal(t, domain.SyncSynced, synced.SyncStatus())

	now := time.Date(2026, 6, 27, 2, 0, 0, 0, time.UTC)
	updated, err := synced.Update(domain.UpdateInput{
		Name:       "checkout SLO v2",
		Service:    "checkout",
		SLIType:    domain.SLIAvailability,
		Target:     0.995,
		WindowDays: 30,
	}, now)

	require.NoError(t, err)
	require.Equal(t, "checkout SLO v2", updated.Name())
	require.Equal(t, 0.995, updated.Target())
	require.Equal(t, domain.SyncPending, updated.SyncStatus())
	require.Equal(t, now, updated.UpdatedAt())
	// bản gốc không đổi (immutability)
	require.Equal(t, domain.SyncSynced, synced.SyncStatus())
}

func TestSLOMarkSyncError(t *testing.T) {
	s, err := domain.NewSLO(giveValidInput())
	require.NoError(t, err)

	got := s.MarkSyncError("reload failed", time.Date(2026, 6, 27, 3, 0, 0, 0, time.UTC))

	require.Equal(t, domain.SyncError, got.SyncStatus())
	require.Equal(t, "reload failed", got.SyncErrorMessage())
}

func TestReconstruct(t *testing.T) {
	created := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)

	got := domain.Reconstruct(domain.ReconstructInput{
		ID:          mustID(),
		WorkspaceID: "ws-1",
		Name:        "api latency",
		Service:     "api",
		SLIType:     domain.SLILatency,
		LatencyMs:   250,
		Target:      0.99,
		WindowDays:  28,
		SyncStatus:  domain.SyncSynced,
		CreatedAt:   created,
		UpdatedAt:   updated,
	})

	require.Equal(t, "api latency", got.Name())
	require.Equal(t, 250, got.LatencyThresholdMs())
	require.Equal(t, domain.SyncSynced, got.SyncStatus())
	require.Equal(t, updated, got.UpdatedAt())
}

func requireValidation(t *testing.T, err error, field string) {
	t.Helper()
	require.Error(t, err)
	var ve *domain.ValidationError
	require.True(t, errors.As(err, &ve), "expected ValidationError, got %v", err)
	require.Equal(t, field, ve.Field)
}
