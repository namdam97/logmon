package command_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/incident/app/command"
	"github.com/namdam97/logmon/backend/internal/incident/domain"
	"github.com/namdam97/logmon/backend/internal/incident/ports"
)

// ---- fakes on-call/escalation ----

type fakeScheduleStore struct{ items map[string]domain.Schedule }

func newFakeScheduleStore() *fakeScheduleStore {
	return &fakeScheduleStore{items: map[string]domain.Schedule{}}
}

func (s *fakeScheduleStore) Save(_ context.Context, sch domain.Schedule) error {
	s.items[sch.ID().String()] = sch
	return nil
}

func (s *fakeScheduleStore) Update(_ context.Context, sch domain.Schedule) error {
	s.items[sch.ID().String()] = sch
	return nil
}

func (s *fakeScheduleStore) ByID(_ context.Context, id domain.ScheduleID) (domain.Schedule, error) {
	sch, ok := s.items[id.String()]
	if !ok {
		return domain.Schedule{}, domain.ErrScheduleNotFound
	}
	return sch, nil
}

func (s *fakeScheduleStore) List(_ context.Context, ws string) ([]domain.Schedule, error) {
	var out []domain.Schedule
	for _, sch := range s.items {
		if sch.WorkspaceID() == ws {
			out = append(out, sch)
		}
	}
	return out, nil
}

type fakeOverrideStore struct{ items []domain.Override }

func (o *fakeOverrideStore) Save(_ context.Context, ov domain.Override) error {
	o.items = append(o.items, ov)
	return nil
}

func (o *fakeOverrideStore) ActiveForSchedule(_ context.Context, sid domain.ScheduleID, at time.Time) ([]domain.Override, error) {
	var out []domain.Override
	for _, ov := range o.items {
		if ov.ScheduleID() == sid && !at.Before(ov.StartAt()) && at.Before(ov.EndAt()) {
			out = append(out, ov)
		}
	}
	return out, nil
}

type fakePolicyStore struct {
	items map[string]domain.EscalationPolicy
}

func newFakePolicyStore() *fakePolicyStore {
	return &fakePolicyStore{items: map[string]domain.EscalationPolicy{}}
}

func (p *fakePolicyStore) Save(_ context.Context, pol domain.EscalationPolicy) error {
	p.items[pol.WorkspaceID()] = pol
	return nil
}

func (p *fakePolicyStore) ByWorkspace(_ context.Context, ws string) (domain.EscalationPolicy, error) {
	pol, ok := p.items[ws]
	if !ok {
		return domain.EscalationPolicy{}, domain.ErrEscalationPolicyNotFound
	}
	return pol, nil
}

type fakeEscState struct{ highest map[string]int }

func newFakeEscState() *fakeEscState { return &fakeEscState{highest: map[string]int{}} }

func (s *fakeEscState) HighestNotified(_ context.Context, id domain.IncidentID) (int, error) {
	v, ok := s.highest[id.String()]
	if !ok {
		return -1, nil
	}
	return v, nil
}

func (s *fakeEscState) Record(_ context.Context, id domain.IncidentID, level int, _ string, _ time.Time) error {
	if cur, ok := s.highest[id.String()]; !ok || level > cur {
		s.highest[id.String()] = level
	}
	return nil
}

type fakeEscNotifier struct{ notices []ports.EscalationNotice }

func (n *fakeEscNotifier) Notify(_ context.Context, notice ports.EscalationNotice) error {
	n.notices = append(n.notices, notice)
	return nil
}

type fakeUnacked struct{ items []domain.Incident }

func (u *fakeUnacked) ListUnacked(_ context.Context) ([]domain.Incident, error) {
	return u.items, nil
}

func mkIncident(t *testing.T, id, ws string, createdAt time.Time) domain.Incident {
	t.Helper()
	iid, err := domain.NewIncidentID(id)
	require.NoError(t, err)
	inc, err := domain.NewIncident(domain.NewIncidentInput{
		ID:          iid,
		WorkspaceID: ws,
		Title:       "db down",
		Service:     "orders",
		Source:      domain.SourceAlert,
		SourceRef:   "fp-1",
		CreatedAt:   createdAt,
	})
	require.NoError(t, err)
	return inc
}

func mkSchedule(t *testing.T, store *fakeScheduleStore, ws string, participants []string) domain.Schedule {
	t.Helper()
	s, err := domain.NewSchedule(domain.NewScheduleInput{
		ID:           "sched-" + ws,
		WorkspaceID:  ws,
		Name:         "oncall",
		Rotation:     "weekly",
		Participants: participants,
		HandoffHour:  9,
		StartDate:    time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
		Now:          time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.NoError(t, store.Save(context.Background(), s))
	return s
}

// ---- command handler tests ----

func TestCreateScheduleHandler(t *testing.T) {
	store := newFakeScheduleStore()
	h := command.NewCreateScheduleHandler(store, &seqIDs{}, &fixedClock{t: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)})

	s, err := h.Handle(context.Background(), command.CreateScheduleInput{
		WorkspaceID:  "ws-1",
		Name:         "platform",
		Rotation:     "weekly",
		Participants: []string{"alice", "bob"},
		HandoffHour:  9,
		StartDate:    time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.Len(t, store.items, 1)
	require.Equal(t, "ws-1", s.WorkspaceID())

	_, err = h.Handle(context.Background(), command.CreateScheduleInput{
		WorkspaceID: "ws-1", Name: "bad", Rotation: "hourly",
		Participants: []string{"alice"}, StartDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	})
	require.Error(t, err, "rotation sai bị từ chối")
}

func TestCreateOverrideHandler(t *testing.T) {
	store := newFakeScheduleStore()
	mkSchedule(t, store, "ws-1", []string{"alice", "bob"})
	ovStore := &fakeOverrideStore{}
	h := command.NewCreateOverrideHandler(ovStore, store, &seqIDs{}, &fixedClock{t: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)})

	_, err := h.Handle(context.Background(), command.CreateOverrideInput{
		ScheduleID:  "sched-ws-1",
		Participant: "carol",
		StartAt:     time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC),
		EndAt:       time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.Len(t, ovStore.items, 1)

	_, err = h.Handle(context.Background(), command.CreateOverrideInput{
		ScheduleID:  "missing",
		Participant: "carol",
		StartAt:     time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC),
		EndAt:       time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC),
	})
	require.ErrorIs(t, err, domain.ErrScheduleNotFound)
}

func TestCreateEscalationPolicyDefault(t *testing.T) {
	store := newFakePolicyStore()
	h := command.NewCreateEscalationPolicyHandler(store, &seqIDs{})

	p, err := h.Handle(context.Background(), command.CreateEscalationPolicyInput{
		WorkspaceID: "ws-1", Name: "default", TeamLead: "lead-1",
	})
	require.NoError(t, err)
	require.Len(t, p.Levels(), 3, "rỗng levels → policy mặc định 3 bậc")
	require.Equal(t, "lead-1", p.TeamLead())
	require.Len(t, store.items, 1)
}

// ---- escalation sweep tests ----

type escHarness struct {
	svc     *command.EscalationService
	unacked *fakeUnacked
	state   *fakeEscState
	notif   *fakeEscNotifier
	clock   *fixedClock
}

func newEscHarness(t *testing.T, withSchedule bool, participants []string) *escHarness {
	t.Helper()
	schedStore := newFakeScheduleStore()
	if withSchedule {
		mkSchedule(t, schedStore, "ws-1", participants)
	}
	polStore := newFakePolicyStore()
	pol, err := domain.DefaultEscalationPolicy("pol-1", "ws-1", "default", "lead-1")
	require.NoError(t, err)
	require.NoError(t, polStore.Save(context.Background(), pol))

	unacked := &fakeUnacked{}
	state := newFakeEscState()
	notif := &fakeEscNotifier{}
	clock := &fixedClock{}
	svc := command.NewEscalationService(unacked, polStore, schedStore, &fakeOverrideStore{}, state, state, notif, clock)
	return &escHarness{svc: svc, unacked: unacked, state: state, notif: notif, clock: clock}
}

func TestEscalationSweepProgressive(t *testing.T) {
	h := newEscHarness(t, true, []string{"alice", "bob"})
	created := time.Date(2026, 6, 27, 10, 0, 0, 0, time.UTC)
	h.unacked.items = []domain.Incident{mkIncident(t, "inc-1", "ws-1", created)}

	// t+20m: primary(t0) + secondary(t+15m) tới hạn, team_lead(t+45m) chưa.
	h.clock.t = created.Add(20 * time.Minute)
	res, err := h.svc.Sweep(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, res.Scanned)
	require.Equal(t, 2, res.Escalated)
	require.Len(t, h.notif.notices, 2)
	require.Equal(t, "alice", h.notif.notices[0].Recipient)
	require.Equal(t, "primary", h.notif.notices[0].Target)
	require.Equal(t, "bob", h.notif.notices[1].Recipient)
	require.Equal(t, "secondary", h.notif.notices[1].Target)

	// Sweep lại cùng thời điểm: không có bậc mới (idempotent).
	res, err = h.svc.Sweep(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0, res.Escalated)
	require.Len(t, h.notif.notices, 2)

	// t+50m: team_lead tới hạn → thêm 1 thông báo.
	h.clock.t = created.Add(50 * time.Minute)
	res, err = h.svc.Sweep(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, res.Escalated)
	require.Len(t, h.notif.notices, 3)
	require.Equal(t, "lead-1", h.notif.notices[2].Recipient)
	require.Equal(t, "team_lead", h.notif.notices[2].Target)
}

func TestEscalationNoScheduleTeamLeadStillFires(t *testing.T) {
	h := newEscHarness(t, false, nil) // không có schedule → primary/secondary rỗng
	created := time.Date(2026, 6, 27, 10, 0, 0, 0, time.UTC)
	h.unacked.items = []domain.Incident{mkIncident(t, "inc-1", "ws-1", created)}

	h.clock.t = created.Add(50 * time.Minute) // mọi bậc tới hạn
	res, err := h.svc.Sweep(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, res.Escalated, "chỉ team_lead resolve được")
	require.Len(t, h.notif.notices, 1)
	require.Equal(t, "team_lead", h.notif.notices[0].Target)
	// Con trỏ vẫn advance qua bậc 0,1 để không kẹt.
	hn, err := h.state.HighestNotified(context.Background(), mustIncidentID(t, "inc-1"))
	require.NoError(t, err)
	require.Equal(t, 2, hn)
}

func TestEscalationSkipsBeforeFirstMark(t *testing.T) {
	h := newEscHarness(t, true, []string{"alice", "bob"})
	created := time.Date(2026, 6, 27, 10, 0, 0, 0, time.UTC)
	h.unacked.items = []domain.Incident{mkIncident(t, "inc-1", "ws-1", created)}

	h.clock.t = created // elapsed 0 → chỉ primary (mark 0)
	res, err := h.svc.Sweep(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, res.Escalated)
	require.Equal(t, "primary", h.notif.notices[0].Target)
}

func TestEscalationNoPolicySkips(t *testing.T) {
	h := newEscHarness(t, true, []string{"alice"})
	created := time.Date(2026, 6, 27, 10, 0, 0, 0, time.UTC)
	// incident thuộc workspace khác (không có policy).
	h.unacked.items = []domain.Incident{mkIncident(t, "inc-9", "ws-other", created)}

	h.clock.t = created.Add(2 * time.Hour)
	res, err := h.svc.Sweep(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, res.Scanned)
	require.Equal(t, 0, res.Escalated, "workspace chưa cấu hình escalation → bỏ qua")
}

func mustIncidentID(t *testing.T, raw string) domain.IncidentID {
	t.Helper()
	id, err := domain.NewIncidentID(raw)
	require.NoError(t, err)
	return id
}
