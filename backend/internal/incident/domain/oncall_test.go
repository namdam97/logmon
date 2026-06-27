package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/incident/domain"
)

// newTestSchedule là helper dựng schedule weekly 3 người, anchor 2026-01-05 09:00 UTC.
func newTestSchedule(t *testing.T, rotation string, participants []string, tz string) domain.Schedule {
	t.Helper()
	s, err := domain.NewSchedule(domain.NewScheduleInput{
		ID:           "sched-1",
		WorkspaceID:  "ws-1",
		Name:         "platform-oncall",
		Rotation:     rotation,
		Participants: participants,
		Timezone:     tz,
		HandoffHour:  9,
		HandoffMin:   0,
		StartDate:    time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
		Now:          time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	return s
}

func TestNewScheduleValidation(t *testing.T) {
	base := domain.NewScheduleInput{
		ID:           "s1",
		WorkspaceID:  "ws-1",
		Name:         "oncall",
		Rotation:     "weekly",
		Participants: []string{"alice"},
		HandoffHour:  9,
		StartDate:    time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
		Now:          time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	tests := []struct {
		name    string
		mutate  func(in *domain.NewScheduleInput)
		wantErr bool
	}{
		{name: "valid", mutate: func(*domain.NewScheduleInput) {}},
		{name: "empty id", mutate: func(in *domain.NewScheduleInput) { in.ID = "" }, wantErr: true},
		{name: "empty workspace", mutate: func(in *domain.NewScheduleInput) { in.WorkspaceID = "" }, wantErr: true},
		{name: "empty name", mutate: func(in *domain.NewScheduleInput) { in.Name = "  " }, wantErr: true},
		{name: "bad rotation", mutate: func(in *domain.NewScheduleInput) { in.Rotation = "hourly" }, wantErr: true},
		{name: "no participants", mutate: func(in *domain.NewScheduleInput) { in.Participants = []string{"  "} }, wantErr: true},
		{name: "bad handoff hour", mutate: func(in *domain.NewScheduleInput) { in.HandoffHour = 24 }, wantErr: true},
		{name: "bad handoff min", mutate: func(in *domain.NewScheduleInput) { in.HandoffMin = 60 }, wantErr: true},
		{name: "bad timezone", mutate: func(in *domain.NewScheduleInput) { in.Timezone = "Mars/Phobos" }, wantErr: true},
		{name: "zero start date", mutate: func(in *domain.NewScheduleInput) { in.StartDate = time.Time{} }, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			give := base
			give.Participants = append([]string(nil), base.Participants...)
			tt.mutate(&give)
			_, err := domain.NewSchedule(give)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestWhoIsOnCallWeeklyRotation(t *testing.T) {
	s := newTestSchedule(t, "weekly", []string{"alice", "bob", "carol"}, "UTC")
	tests := []struct {
		name          string
		at            time.Time
		wantPrimary   string
		wantSecondary string
	}{
		{name: "before anchor → first", at: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), wantPrimary: "alice", wantSecondary: "bob"},
		{name: "week 0", at: time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC), wantPrimary: "alice", wantSecondary: "bob"},
		{name: "week 1", at: time.Date(2026, 1, 12, 10, 0, 0, 0, time.UTC), wantPrimary: "bob", wantSecondary: "carol"},
		{name: "week 2 wraps secondary", at: time.Date(2026, 1, 19, 10, 0, 0, 0, time.UTC), wantPrimary: "carol", wantSecondary: "alice"},
		{name: "week 3 wraps primary", at: time.Date(2026, 1, 26, 10, 0, 0, 0, time.UTC), wantPrimary: "alice", wantSecondary: "bob"},
		{name: "just before handoff stays in week", at: time.Date(2026, 1, 12, 8, 59, 0, 0, time.UTC), wantPrimary: "alice", wantSecondary: "bob"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domain.WhoIsOnCall(s, nil, tt.at)
			require.Equal(t, tt.wantPrimary, got.Primary)
			require.Equal(t, tt.wantSecondary, got.Secondary)
			require.Empty(t, got.OverrideID)
		})
	}
}

func TestWhoIsOnCallDailyRotation(t *testing.T) {
	s := newTestSchedule(t, "daily", []string{"alice", "bob"}, "UTC")
	day0 := domain.WhoIsOnCall(s, nil, time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC))
	require.Equal(t, "alice", day0.Primary)
	require.Equal(t, "bob", day0.Secondary)

	day1 := domain.WhoIsOnCall(s, nil, time.Date(2026, 1, 6, 10, 0, 0, 0, time.UTC))
	require.Equal(t, "bob", day1.Primary)
	require.Equal(t, "alice", day1.Secondary)
}

func TestWhoIsOnCallSingleParticipant(t *testing.T) {
	s := newTestSchedule(t, "daily", []string{"solo"}, "UTC")
	got := domain.WhoIsOnCall(s, nil, time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC))
	require.Equal(t, "solo", got.Primary)
	require.Empty(t, got.Secondary, "không có secondary khi chỉ 1 người")
}

func TestWhoIsOnCallTimezoneHandoff(t *testing.T) {
	// Handoff 09:00 giờ Việt Nam (UTC+7). 01:00 UTC ngày 2026-01-06 = 08:00 VN → vẫn ngày 5.
	s := newTestSchedule(t, "daily", []string{"alice", "bob"}, "Asia/Ho_Chi_Minh")
	beforeHandoff := domain.WhoIsOnCall(s, nil, time.Date(2026, 1, 6, 1, 0, 0, 0, time.UTC))
	require.Equal(t, "alice", beforeHandoff.Primary, "trước 09:00 VN vẫn là ngày của alice")

	afterHandoff := domain.WhoIsOnCall(s, nil, time.Date(2026, 1, 6, 3, 0, 0, 0, time.UTC)) // 10:00 VN
	require.Equal(t, "bob", afterHandoff.Primary, "sau 09:00 VN đã bàn giao cho bob")
}

func TestWhoIsOnCallWithOverride(t *testing.T) {
	s := newTestSchedule(t, "weekly", []string{"alice", "bob", "carol"}, "UTC")
	ov, err := domain.NewOverride(domain.NewOverrideInput{
		ID:          "ov-1",
		ScheduleID:  "sched-1",
		Participant: "dave",
		StartAt:     time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
		EndAt:       time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC),
		Now:         time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	// Trong khoảng override: primary = dave, override id ghi nhận.
	covered := domain.WhoIsOnCall(s, []domain.Override{ov}, time.Date(2026, 1, 6, 10, 0, 0, 0, time.UTC))
	require.Equal(t, "dave", covered.Primary)
	require.Equal(t, "ov-1", covered.OverrideID)
	require.Equal(t, "bob", covered.Secondary, "secondary vẫn theo rotation")

	// Ngoài khoảng override: quay lại rotation thường.
	after := domain.WhoIsOnCall(s, []domain.Override{ov}, time.Date(2026, 1, 8, 10, 0, 0, 0, time.UTC))
	require.Equal(t, "alice", after.Primary)
	require.Empty(t, after.OverrideID)
}

func TestWhoIsOnCallOverlappingOverridesLatestWins(t *testing.T) {
	s := newTestSchedule(t, "weekly", []string{"alice"}, "UTC")
	mk := func(id, who string, start time.Time) domain.Override {
		o, err := domain.NewOverride(domain.NewOverrideInput{
			ID:          id,
			ScheduleID:  "sched-1",
			Participant: who,
			StartAt:     start,
			EndAt:       start.Add(48 * time.Hour),
			Now:         time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		})
		require.NoError(t, err)
		return o
	}
	early := mk("ov-early", "dave", time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC))
	late := mk("ov-late", "erin", time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC))

	at := time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC) // cả hai cùng phủ
	got := domain.WhoIsOnCall(s, []domain.Override{early, late}, at)
	require.Equal(t, "erin", got.Primary, "override StartAt mới nhất thắng")
	require.Equal(t, "ov-late", got.OverrideID)
}

func TestNewOverrideValidation(t *testing.T) {
	base := domain.NewOverrideInput{
		ID:          "ov",
		ScheduleID:  "s1",
		Participant: "dave",
		StartAt:     time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
		EndAt:       time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC),
		Now:         time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	tests := []struct {
		name    string
		mutate  func(in *domain.NewOverrideInput)
		wantErr bool
	}{
		{name: "valid", mutate: func(*domain.NewOverrideInput) {}},
		{name: "empty id", mutate: func(in *domain.NewOverrideInput) { in.ID = "" }, wantErr: true},
		{name: "empty participant", mutate: func(in *domain.NewOverrideInput) { in.Participant = "" }, wantErr: true},
		{name: "end before start", mutate: func(in *domain.NewOverrideInput) { in.EndAt = in.StartAt.Add(-time.Hour) }, wantErr: true},
		{name: "end equals start", mutate: func(in *domain.NewOverrideInput) { in.EndAt = in.StartAt }, wantErr: true},
		{name: "zero start", mutate: func(in *domain.NewOverrideInput) { in.StartAt = time.Time{} }, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			give := base
			tt.mutate(&give)
			_, err := domain.NewOverride(give)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestScheduleParticipantsCopy(t *testing.T) {
	s := newTestSchedule(t, "weekly", []string{"alice", "bob"}, "UTC")
	got := s.Participants()
	got[0] = "mutated"
	require.Equal(t, "alice", s.Participants()[0], "Participants() trả bản sao, không lộ slice nội bộ")
}
