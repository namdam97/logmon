package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/incident/domain"
)

func TestDefaultEscalationPolicy(t *testing.T) {
	p, err := domain.DefaultEscalationPolicy("p1", "ws-1", "default", "lead-1")
	require.NoError(t, err)
	require.Len(t, p.Levels(), 3)
	require.Equal(t, "lead-1", p.TeamLead())

	levels := p.Levels()
	require.Equal(t, domain.TargetPrimary, levels[0].Target())
	require.Equal(t, 15*time.Minute, levels[0].Timeout())
	require.Equal(t, domain.TargetSecondary, levels[1].Target())
	require.Equal(t, 30*time.Minute, levels[1].Timeout())
	require.Equal(t, domain.TargetTeamLead, levels[2].Target())
	require.Equal(t, 60*time.Minute, levels[2].Timeout())
}

func TestEscalationPolicyValidation(t *testing.T) {
	lvls := []domain.EscalationLevel{
		mustLevel(t, "primary", 15*time.Minute),
		mustLevel(t, "team_lead", 60*time.Minute),
	}
	tests := []struct {
		name    string
		in      domain.NewEscalationPolicyInput
		wantErr bool
	}{
		{
			name: "valid",
			in:   domain.NewEscalationPolicyInput{ID: "p", WorkspaceID: "ws", Name: "n", Levels: lvls, TeamLead: "lead"},
		},
		{
			name:    "empty id",
			in:      domain.NewEscalationPolicyInput{ID: "", WorkspaceID: "ws", Name: "n", Levels: lvls, TeamLead: "lead"},
			wantErr: true,
		},
		{
			name:    "no levels",
			in:      domain.NewEscalationPolicyInput{ID: "p", WorkspaceID: "ws", Name: "n", Levels: nil, TeamLead: "lead"},
			wantErr: true,
		},
		{
			name:    "team_lead level but no team lead set",
			in:      domain.NewEscalationPolicyInput{ID: "p", WorkspaceID: "ws", Name: "n", Levels: lvls, TeamLead: ""},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewEscalationPolicy(tt.in)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestNewEscalationLevelValidation(t *testing.T) {
	_, err := domain.NewEscalationLevel("primary", 15*time.Minute)
	require.NoError(t, err)
	_, err = domain.NewEscalationLevel("ceo", 15*time.Minute)
	require.Error(t, err)
	_, err = domain.NewEscalationLevel("primary", 0)
	require.Error(t, err)
}

func TestDueLevelIndexes(t *testing.T) {
	// primary(15m) → secondary(30m) → team_lead(1h).
	// Mốc thông báo: primary=0, secondary=15m, team_lead=45m.
	p, err := domain.DefaultEscalationPolicy("p1", "ws-1", "default", "lead-1")
	require.NoError(t, err)
	tests := []struct {
		name        string
		elapsed     time.Duration
		wantDue     []int
		wantHighest int
	}{
		{name: "t=0 only primary", elapsed: 0, wantDue: []int{0}, wantHighest: 0},
		{name: "t=10m still primary", elapsed: 10 * time.Minute, wantDue: []int{0}, wantHighest: 0},
		{name: "t=15m secondary due", elapsed: 15 * time.Minute, wantDue: []int{0, 1}, wantHighest: 1},
		{name: "t=44m not team_lead yet", elapsed: 44 * time.Minute, wantDue: []int{0, 1}, wantHighest: 1},
		{name: "t=45m team_lead due", elapsed: 45 * time.Minute, wantDue: []int{0, 1, 2}, wantHighest: 2},
		{name: "t=2h all due", elapsed: 2 * time.Hour, wantDue: []int{0, 1, 2}, wantHighest: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.wantDue, p.DueLevelIndexes(tt.elapsed))
			require.Equal(t, tt.wantHighest, p.HighestDueLevel(tt.elapsed))
		})
	}
}

func TestHighestDueLevelNegativeElapsed(t *testing.T) {
	p, err := domain.DefaultEscalationPolicy("p1", "ws-1", "default", "lead-1")
	require.NoError(t, err)
	require.Equal(t, -1, p.HighestDueLevel(-time.Minute))
}

func TestResolveTarget(t *testing.T) {
	p, err := domain.DefaultEscalationPolicy("p1", "ws-1", "default", "lead-1")
	require.NoError(t, err)
	oncall := domain.OnCall{Primary: "alice", Secondary: "bob"}

	got, ok := p.ResolveTarget(0, oncall)
	require.True(t, ok)
	require.Equal(t, "alice", got)

	got, ok = p.ResolveTarget(1, oncall)
	require.True(t, ok)
	require.Equal(t, "bob", got)

	got, ok = p.ResolveTarget(2, oncall)
	require.True(t, ok)
	require.Equal(t, "lead-1", got)

	_, ok = p.ResolveTarget(99, oncall)
	require.False(t, ok, "index ngoài phạm vi")
}

func TestResolveTargetEmptySecondary(t *testing.T) {
	p, err := domain.DefaultEscalationPolicy("p1", "ws-1", "default", "lead-1")
	require.NoError(t, err)
	oncall := domain.OnCall{Primary: "solo"} // không có secondary
	_, ok := p.ResolveTarget(1, oncall)
	require.False(t, ok, "secondary rỗng không resolve được")
}

func mustLevel(t *testing.T, target string, timeout time.Duration) domain.EscalationLevel {
	t.Helper()
	l, err := domain.NewEscalationLevel(target, timeout)
	require.NoError(t, err)
	return l
}
