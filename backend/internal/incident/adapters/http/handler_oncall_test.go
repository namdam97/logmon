package http

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/incident/app/command"
	"github.com/namdam97/logmon/backend/internal/incident/domain"
)

type stubScheduleCreator struct {
	s   domain.Schedule
	err error
}

func (s stubScheduleCreator) Handle(context.Context, command.CreateScheduleInput) (domain.Schedule, error) {
	return s.s, s.err
}

type stubOverrideCreator struct {
	o   domain.Override
	err error
}

func (s stubOverrideCreator) Handle(context.Context, command.CreateOverrideInput) (domain.Override, error) {
	return s.o, s.err
}

type stubPolicyCreator struct {
	p   domain.EscalationPolicy
	err error
}

func (s stubPolicyCreator) Handle(context.Context, command.CreateEscalationPolicyInput) (domain.EscalationPolicy, error) {
	return s.p, s.err
}

type stubOnCallQueries struct {
	schedules []domain.Schedule
	sched     domain.Schedule
	oncall    domain.OnCall
	err       error
}

func (s stubOnCallQueries) ListSchedules(context.Context, string) ([]domain.Schedule, error) {
	return s.schedules, s.err
}

func (s stubOnCallQueries) Current(context.Context, string, string, time.Time) (domain.Schedule, domain.OnCall, error) {
	return s.sched, s.oncall, s.err
}

func sampleSchedule(t *testing.T) domain.Schedule {
	t.Helper()
	s, err := domain.NewSchedule(domain.NewScheduleInput{
		ID:           "sched-1",
		WorkspaceID:  _ws,
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

func newOnCallRouter(h *OnCallHandler) *gin.Engine {
	r := gin.New()
	api := r.Group("/api/v1")
	h.Register(api, func(c *gin.Context) { c.Set("auth_role", "admin"); c.Next() })
	return r
}

func TestCreateScheduleHTTP(t *testing.T) {
	h := NewOnCallHandler(stubScheduleCreator{s: sampleSchedule(t)}, stubOverrideCreator{}, stubPolicyCreator{}, stubOnCallQueries{}, _ws)
	w := doJSON(t, newOnCallRouter(h), http.MethodPost, "/api/v1/oncall/schedules",
		`{"name":"oncall","rotation":"weekly","participants":["alice","bob"],"handoffHour":9,"startDate":"2026-01-05"}`)
	require.Equal(t, http.StatusCreated, w.Code)

	var env struct {
		Data scheduleResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.Equal(t, "weekly", env.Data.Rotation)
	require.Equal(t, []string{"alice", "bob"}, env.Data.Participants)
}

func TestCreateScheduleBadDate(t *testing.T) {
	h := NewOnCallHandler(stubScheduleCreator{s: sampleSchedule(t)}, stubOverrideCreator{}, stubPolicyCreator{}, stubOnCallQueries{}, _ws)
	w := doJSON(t, newOnCallRouter(h), http.MethodPost, "/api/v1/oncall/schedules",
		`{"name":"oncall","rotation":"weekly","participants":["alice"],"startDate":"not-a-date"}`)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCurrentOnCallHTTP(t *testing.T) {
	h := NewOnCallHandler(stubScheduleCreator{}, stubOverrideCreator{}, stubPolicyCreator{},
		stubOnCallQueries{oncall: domain.OnCall{Primary: "alice", Secondary: "bob"}}, _ws)
	w := doJSON(t, newOnCallRouter(h), http.MethodGet,
		"/api/v1/oncall/schedules/sched-1/current?at=2026-01-05T10:00:00Z", "")
	require.Equal(t, http.StatusOK, w.Code)

	var env struct {
		Data onCallResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.Equal(t, "alice", env.Data.Primary)
	require.Equal(t, "bob", env.Data.Secondary)
}

func TestCurrentOnCallScheduleNotFound(t *testing.T) {
	h := NewOnCallHandler(stubScheduleCreator{}, stubOverrideCreator{}, stubPolicyCreator{},
		stubOnCallQueries{err: domain.ErrScheduleNotFound}, _ws)
	w := doJSON(t, newOnCallRouter(h), http.MethodGet, "/api/v1/oncall/schedules/missing/current", "")
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestCreateOverrideHTTP(t *testing.T) {
	ov, err := domain.NewOverride(domain.NewOverrideInput{
		ID: "ov-1", ScheduleID: "sched-1", Participant: "carol",
		StartAt: time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC),
		EndAt:   time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC),
		Now:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	h := NewOnCallHandler(stubScheduleCreator{}, stubOverrideCreator{o: ov}, stubPolicyCreator{}, stubOnCallQueries{}, _ws)
	w := doJSON(t, newOnCallRouter(h), http.MethodPost, "/api/v1/oncall/override",
		`{"scheduleId":"sched-1","participant":"carol","startAt":"2026-01-06T00:00:00Z","endAt":"2026-01-07T00:00:00Z"}`)
	require.Equal(t, http.StatusCreated, w.Code)
}

func TestCreateOverrideBadTime(t *testing.T) {
	h := NewOnCallHandler(stubScheduleCreator{}, stubOverrideCreator{}, stubPolicyCreator{}, stubOnCallQueries{}, _ws)
	w := doJSON(t, newOnCallRouter(h), http.MethodPost, "/api/v1/oncall/override",
		`{"scheduleId":"sched-1","participant":"carol","startAt":"bad","endAt":"bad"}`)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreatePolicyHTTP(t *testing.T) {
	p, err := domain.DefaultEscalationPolicy("pol-1", _ws, "default", "lead-1")
	require.NoError(t, err)
	h := NewOnCallHandler(stubScheduleCreator{}, stubOverrideCreator{}, stubPolicyCreator{p: p}, stubOnCallQueries{}, _ws)
	w := doJSON(t, newOnCallRouter(h), http.MethodPost, "/api/v1/oncall/escalation-policy",
		`{"name":"default","teamLead":"lead-1"}`)
	require.Equal(t, http.StatusCreated, w.Code)

	var env struct {
		Data policyResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.Len(t, env.Data.Levels, 3)
}

func TestListSchedulesHTTP(t *testing.T) {
	h := NewOnCallHandler(stubScheduleCreator{}, stubOverrideCreator{}, stubPolicyCreator{},
		stubOnCallQueries{schedules: []domain.Schedule{sampleSchedule(t)}}, _ws)
	w := doJSON(t, newOnCallRouter(h), http.MethodGet, "/api/v1/oncall/schedules", "")
	require.Equal(t, http.StatusOK, w.Code)

	var env struct {
		Data []scheduleResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.Len(t, env.Data, 1)
}
