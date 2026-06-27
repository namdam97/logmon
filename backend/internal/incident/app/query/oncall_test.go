package query_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/incident/app/query"
	"github.com/namdam97/logmon/backend/internal/incident/domain"
)

type fakeScheduleReader struct{ items map[string]domain.Schedule }

func (s *fakeScheduleReader) ByID(_ context.Context, id domain.ScheduleID) (domain.Schedule, error) {
	sch, ok := s.items[id.String()]
	if !ok {
		return domain.Schedule{}, domain.ErrScheduleNotFound
	}
	return sch, nil
}

func (s *fakeScheduleReader) List(_ context.Context, ws string) ([]domain.Schedule, error) {
	var out []domain.Schedule
	for _, sch := range s.items {
		if sch.WorkspaceID() == ws {
			out = append(out, sch)
		}
	}
	return out, nil
}

type fakeOverrideReader struct{ items []domain.Override }

func (o *fakeOverrideReader) ActiveForSchedule(_ context.Context, sid domain.ScheduleID, at time.Time) ([]domain.Override, error) {
	var out []domain.Override
	for _, ov := range o.items {
		if ov.ScheduleID() == sid && !at.Before(ov.StartAt()) && at.Before(ov.EndAt()) {
			out = append(out, ov)
		}
	}
	return out, nil
}

func newSchedule(t *testing.T) domain.Schedule {
	t.Helper()
	s, err := domain.NewSchedule(domain.NewScheduleInput{
		ID:           "sched-1",
		WorkspaceID:  "ws-1",
		Name:         "oncall",
		Rotation:     "weekly",
		Participants: []string{"alice", "bob"},
		HandoffHour:  9,
		StartDate:    time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
		Now:          time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	return s
}

func TestOnCallQueriesCurrent(t *testing.T) {
	s := newSchedule(t)
	sr := &fakeScheduleReader{items: map[string]domain.Schedule{"sched-1": s}}
	or := &fakeOverrideReader{}
	q := query.NewOnCallQueries(sr, or)

	at := time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC) // tuần 0 → alice
	sched, oncall, err := q.Current(context.Background(), "ws-1", "sched-1", at)
	require.NoError(t, err)
	require.Equal(t, "sched-1", sched.ID().String())
	require.Equal(t, "alice", oncall.Primary)
	require.Equal(t, "bob", oncall.Secondary)
}

func TestOnCallQueriesCurrentWrongWorkspace(t *testing.T) {
	s := newSchedule(t)
	sr := &fakeScheduleReader{items: map[string]domain.Schedule{"sched-1": s}}
	q := query.NewOnCallQueries(sr, &fakeOverrideReader{})

	_, _, err := q.Current(context.Background(), "ws-other", "sched-1", time.Now())
	require.ErrorIs(t, err, domain.ErrScheduleNotFound)
}

func TestOnCallQueriesListSchedules(t *testing.T) {
	s := newSchedule(t)
	sr := &fakeScheduleReader{items: map[string]domain.Schedule{"sched-1": s}}
	q := query.NewOnCallQueries(sr, &fakeOverrideReader{})

	list, err := q.ListSchedules(context.Background(), "ws-1")
	require.NoError(t, err)
	require.Len(t, list, 1)
}
