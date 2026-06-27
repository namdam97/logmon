package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/incident/domain"
)

func mustID(t *testing.T, raw string) domain.IncidentID {
	t.Helper()
	id, err := domain.NewIncidentID(raw)
	require.NoError(t, err)
	return id
}

var _baseTime = time.Date(2026, 6, 27, 10, 0, 0, 0, time.UTC)

func newOpenIncident(t *testing.T) domain.Incident {
	t.Helper()
	inc, err := domain.NewIncident(domain.NewIncidentInput{
		ID:          mustID(t, "inc-1"),
		WorkspaceID: "ws-1",
		Title:       "checkout latency spike",
		Service:     "checkout-api",
		Source:      domain.SourceManual,
		CreatedAt:   _baseTime,
	})
	require.NoError(t, err)
	return inc
}

func TestNewIncident(t *testing.T) {
	tests := []struct {
		name string
		give domain.NewIncidentInput
		want string // field lỗi mong đợi; "" = không lỗi
	}{
		{
			name: "valid open incident",
			give: domain.NewIncidentInput{
				ID: mustID(t, "inc-1"), WorkspaceID: "ws-1", Title: "db down",
				Service: "orders", Source: domain.SourceAlert, CreatedAt: _baseTime,
			},
			want: "",
		},
		{
			name: "empty title",
			give: domain.NewIncidentInput{
				ID: mustID(t, "inc-1"), WorkspaceID: "ws-1", Title: "  ",
				Service: "orders", Source: domain.SourceAlert, CreatedAt: _baseTime,
			},
			want: "title",
		},
		{
			name: "invalid service chars",
			give: domain.NewIncidentInput{
				ID: mustID(t, "inc-1"), WorkspaceID: "ws-1", Title: "x",
				Service: "bad service!", Source: domain.SourceAlert, CreatedAt: _baseTime,
			},
			want: "service",
		},
		{
			name: "missing source",
			give: domain.NewIncidentInput{
				ID: mustID(t, "inc-1"), WorkspaceID: "ws-1", Title: "x",
				Service: "orders", CreatedAt: _baseTime,
			},
			want: "source",
		},
		{
			name: "zero createdAt",
			give: domain.NewIncidentInput{
				ID: mustID(t, "inc-1"), WorkspaceID: "ws-1", Title: "x",
				Service: "orders", Source: domain.SourceManual,
			},
			want: "createdAt",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inc, err := domain.NewIncident(tt.give)
			if tt.want == "" {
				require.NoError(t, err)
				require.Equal(t, domain.StatusOpen, inc.Status())
				require.True(t, inc.Severity().IsZero())
				return
			}
			var ve *domain.ValidationError
			require.ErrorAs(t, err, &ve)
			require.Equal(t, tt.want, ve.Field)
		})
	}
}

func TestHappyPathLifecycle(t *testing.T) {
	// Open → Triaged → Assigned → Mitigating → Resolved (SEV2) → PostmortemPending → Closed
	inc := newOpenIncident(t)

	triaged, err := inc.Triage(domain.SEV2, "user-facing degradation", _baseTime.Add(2*time.Minute))
	require.NoError(t, err)
	require.Equal(t, domain.StatusTriaged, triaged.Status())
	require.Equal(t, domain.SEV2, triaged.Severity())

	assigned, err := triaged.Assign("alice", _baseTime.Add(5*time.Minute))
	require.NoError(t, err)
	require.Equal(t, domain.StatusAssigned, assigned.Status())
	require.Equal(t, "alice", assigned.Assignee())

	mtta, ok := assigned.MTTA()
	require.True(t, ok)
	require.Equal(t, 5*time.Minute, mtta)

	mitigating, err := assigned.StartMitigation(_baseTime.Add(6 * time.Minute))
	require.NoError(t, err)
	require.Equal(t, domain.StatusMitigating, mitigating.Status())

	resolved, err := mitigating.Resolve(_baseTime.Add(30 * time.Minute))
	require.NoError(t, err)
	require.Equal(t, domain.StatusResolved, resolved.Status())

	mttr, ok := resolved.MTTR()
	require.True(t, ok)
	require.Equal(t, 30*time.Minute, mttr)

	// SEV2 Resolved KHÔNG được close thẳng — phải qua PostmortemPending.
	_, err = resolved.Close(_baseTime.Add(31 * time.Minute))
	require.ErrorIs(t, err, domain.ErrInvalidTransition)

	pmp, err := resolved.RequirePostmortem(_baseTime.Add(24 * time.Hour))
	require.NoError(t, err)
	require.Equal(t, domain.StatusPostmortemPending, pmp.Status())

	closed, err := pmp.Close(_baseTime.Add(48 * time.Hour))
	require.NoError(t, err)
	require.Equal(t, domain.StatusClosed, closed.Status())
	require.NotNil(t, closed.ClosedAt())
}

func TestSEV3ClosesWithoutPostmortem(t *testing.T) {
	inc := newOpenIncident(t)
	triaged, err := inc.Triage(domain.SEV3, "minor", _baseTime.Add(time.Minute))
	require.NoError(t, err)
	assigned, err := triaged.Assign("bob", _baseTime.Add(2*time.Minute))
	require.NoError(t, err)
	mit, err := assigned.StartMitigation(_baseTime.Add(3 * time.Minute))
	require.NoError(t, err)
	resolved, err := mit.Resolve(_baseTime.Add(10 * time.Minute))
	require.NoError(t, err)

	closed, err := resolved.Close(_baseTime.Add(11 * time.Minute))
	require.NoError(t, err)
	require.Equal(t, domain.StatusClosed, closed.Status())
}

func TestOpenResolveFalseAlarm(t *testing.T) {
	inc := newOpenIncident(t)
	resolved, err := inc.Resolve(_baseTime.Add(time.Minute))
	require.NoError(t, err)
	require.Equal(t, domain.StatusResolved, resolved.Status())
	_, ok := resolved.MTTA()
	require.False(t, ok) // chưa từng assign
}

func TestReassignFromMitigating(t *testing.T) {
	inc := newOpenIncident(t)
	triaged, _ := inc.Triage(domain.SEV1, "", _baseTime.Add(time.Minute))
	assigned, _ := triaged.Assign("alice", _baseTime.Add(2*time.Minute))
	mit, _ := assigned.StartMitigation(_baseTime.Add(3 * time.Minute))

	reassigned, err := mit.Assign("carol", _baseTime.Add(20*time.Minute))
	require.NoError(t, err)
	require.Equal(t, domain.StatusAssigned, reassigned.Status())
	require.Equal(t, "carol", reassigned.Assignee())

	// MTTA giữ nguyên lần assign ĐẦU TIÊN (2 phút), không phải 20 phút.
	mtta, ok := reassigned.MTTA()
	require.True(t, ok)
	require.Equal(t, 2*time.Minute, mtta)
}

func TestInvalidTransitions(t *testing.T) {
	inc := newOpenIncident(t)
	tests := []struct {
		name string
		act  func(domain.Incident) (domain.Incident, error)
	}{
		{"assign from open", func(i domain.Incident) (domain.Incident, error) { return i.Assign("x", _baseTime) }},
		{"mitigate from open", func(i domain.Incident) (domain.Incident, error) { return i.StartMitigation(_baseTime) }},
		{"close from open", func(i domain.Incident) (domain.Incident, error) { return i.Close(_baseTime) }},
		{"postmortem from open", func(i domain.Incident) (domain.Incident, error) { return i.RequirePostmortem(_baseTime) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.act(inc)
			require.ErrorIs(t, err, domain.ErrInvalidTransition)
		})
	}
}

func TestTriageRequiresSeverity(t *testing.T) {
	inc := newOpenIncident(t)
	_, err := inc.Triage(domain.Severity{}, "x", _baseTime)
	var ve *domain.ValidationError
	require.ErrorAs(t, err, &ve)
	require.Equal(t, "severity", ve.Field)
}

func TestImmutability(t *testing.T) {
	inc := newOpenIncident(t)
	_, err := inc.Triage(domain.SEV1, "x", _baseTime.Add(time.Minute))
	require.NoError(t, err)
	// Bản gốc KHÔNG đổi.
	require.Equal(t, domain.StatusOpen, inc.Status())
	require.True(t, inc.Severity().IsZero())
}

func TestSeverityRequiresPostmortem(t *testing.T) {
	require.True(t, domain.SEV1.RequiresPostmortem())
	require.True(t, domain.SEV2.RequiresPostmortem())
	require.False(t, domain.SEV3.RequiresPostmortem())
	require.False(t, domain.SEV4.RequiresPostmortem())
}

func TestSeverityLabel(t *testing.T) {
	require.Equal(t, "untriaged", domain.Severity{}.Label())
	require.Equal(t, "SEV1", domain.SEV1.Label())
}

func TestStatusIsActive(t *testing.T) {
	active := []domain.Status{domain.StatusOpen, domain.StatusTriaged, domain.StatusAssigned, domain.StatusMitigating}
	for _, s := range active {
		require.True(t, s.IsActive(), s.String())
	}
	inactive := []domain.Status{domain.StatusResolved, domain.StatusPostmortemPending, domain.StatusClosed}
	for _, s := range inactive {
		require.False(t, s.IsActive(), s.String())
	}
}

func TestNewSeverityInvalid(t *testing.T) {
	_, err := domain.NewSeverity("SEV9")
	require.Error(t, err)
	require.False(t, errors.Is(err, domain.ErrInvalidTransition))
}
