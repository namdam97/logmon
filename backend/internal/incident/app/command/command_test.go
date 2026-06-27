package command_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/incident/app/command"
	"github.com/namdam97/logmon/backend/internal/incident/domain"
)

// ---- fakes ----

type fakeTx struct{}

func (fakeTx) WithinTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type fakeRepo struct {
	byID map[string]domain.Incident
}

func newFakeRepo() *fakeRepo { return &fakeRepo{byID: map[string]domain.Incident{}} }

func (r *fakeRepo) Save(_ context.Context, inc domain.Incident) error {
	r.byID[inc.ID().String()] = inc
	return nil
}

func (r *fakeRepo) Update(_ context.Context, inc domain.Incident) error {
	if _, ok := r.byID[inc.ID().String()]; !ok {
		return domain.ErrIncidentNotFound
	}
	r.byID[inc.ID().String()] = inc
	return nil
}

func (r *fakeRepo) ByID(_ context.Context, id domain.IncidentID) (domain.Incident, error) {
	inc, ok := r.byID[id.String()]
	if !ok {
		return domain.Incident{}, domain.ErrIncidentNotFound
	}
	return inc, nil
}

func (r *fakeRepo) List(_ context.Context, ws string) ([]domain.Incident, error) {
	return r.filter(ws, false), nil
}

func (r *fakeRepo) ListActive(_ context.Context, ws string) ([]domain.Incident, error) {
	return r.filter(ws, true), nil
}

func (r *fakeRepo) filter(ws string, activeOnly bool) []domain.Incident {
	var out []domain.Incident
	for _, inc := range r.byID {
		if inc.WorkspaceID() != ws {
			continue
		}
		if activeOnly && !inc.Status().IsActive() {
			continue
		}
		out = append(out, inc)
	}
	return out
}

func (r *fakeRepo) ActiveBySourceRef(_ context.Context, ws string, src domain.Source, ref string) (domain.Incident, error) {
	for _, inc := range r.byID {
		if inc.WorkspaceID() == ws && inc.Source() == src && inc.SourceRef() == ref && inc.Status().IsActive() {
			return inc, nil
		}
	}
	return domain.Incident{}, domain.ErrIncidentNotFound
}

type fakeTimeline struct{ entries []domain.TimelineEntry }

func (t *fakeTimeline) Append(_ context.Context, e domain.TimelineEntry) error {
	t.entries = append(t.entries, e)
	return nil
}

func (t *fakeTimeline) List(_ context.Context, id domain.IncidentID) ([]domain.TimelineEntry, error) {
	var out []domain.TimelineEntry
	for _, e := range t.entries {
		if e.IncidentID() == id.String() {
			out = append(out, e)
		}
	}
	return out, nil
}

type fakePublisher struct{ events []string }

func (p *fakePublisher) Publish(_ context.Context, _, _, eventType string, _ any) error {
	p.events = append(p.events, eventType)
	return nil
}

type fakeMetrics struct {
	opened    int
	retriaged int
	mtta      []time.Duration
	mttr      []time.Duration
}

func (m *fakeMetrics) Opened(_, _ string)    { m.opened++ }
func (m *fakeMetrics) Retriaged(_, _ string) { m.retriaged++ }
func (m *fakeMetrics) Assigned(d time.Duration) {
	m.mtta = append(m.mtta, d)
}
func (m *fakeMetrics) Resolved(_ string, d time.Duration) { m.mttr = append(m.mttr, d) }

type seqIDs struct{ n int }

func (g *seqIDs) NewID() string {
	g.n++
	return "id-" + time.Duration(g.n).String()
}

type fixedClock struct{ t time.Time }

func (c *fixedClock) Now() time.Time { return c.t }

// ---- harness ----

type harness struct {
	create *command.CreateIncidentHandler
	trans  *command.TransitionHandler
	repo   *fakeRepo
	tl     *fakeTimeline
	pub    *fakePublisher
	mx     *fakeMetrics
	clock  *fixedClock
}

func newHarness() *harness {
	repo := newFakeRepo()
	tl := &fakeTimeline{}
	pub := &fakePublisher{}
	mx := &fakeMetrics{}
	clock := &fixedClock{t: time.Date(2026, 6, 27, 10, 0, 0, 0, time.UTC)}
	ids := &seqIDs{}
	tx := fakeTx{}
	return &harness{
		create: command.NewCreateIncidentHandler(tx, repo, repo, tl, pub, mx, ids, clock),
		trans:  command.NewTransitionHandler(tx, repo, repo, tl, pub, mx, ids, clock),
		repo:   repo, tl: tl, pub: pub, mx: mx, clock: clock,
	}
}

func (h *harness) createOpen(t *testing.T) domain.Incident {
	t.Helper()
	inc, err := h.create.Handle(context.Background(), command.CreateIncidentInput{
		WorkspaceID: "ws-1", Title: "db latency", Service: "orders",
		Source: "manual", Actor: "operator",
	})
	require.NoError(t, err)
	return inc
}

func TestCreateIncident(t *testing.T) {
	h := newHarness()
	inc := h.createOpen(t)

	require.Equal(t, domain.StatusOpen, inc.Status())
	require.Equal(t, 1, h.mx.opened)
	require.Equal(t, []string{domain.EventIncidentCreated}, h.pub.events)
	require.Len(t, h.tl.entries, 1)
	require.Equal(t, domain.KindCreated, h.tl.entries[0].Kind())
}

func TestAutoCreateDedup(t *testing.T) {
	h := newHarness()
	in := command.CreateIncidentInput{
		WorkspaceID: "ws-1", Title: "budget exhausted", Service: "orders",
		Severity: "SEV2", Source: "slo_budget", SourceRef: "slo-42", Actor: "system",
	}
	first, err := h.create.Handle(context.Background(), in)
	require.NoError(t, err)

	second, err := h.create.Handle(context.Background(), in)
	require.NoError(t, err)

	require.Equal(t, first.ID().String(), second.ID().String()) // tái dùng, không tạo mới
	require.Len(t, h.repo.byID, 1)
	require.Equal(t, 1, h.mx.opened) // Opened chỉ gọi 1 lần
}

func TestFullTransitionFlow(t *testing.T) {
	h := newHarness()
	inc := h.createOpen(t)
	ws, id := "ws-1", inc.ID().String()

	h.clock.t = h.clock.t.Add(2 * time.Minute)
	_, err := h.trans.Triage(context.Background(), ws, id, "SEV2", "user impact", "alice")
	require.NoError(t, err)
	require.Equal(t, 1, h.mx.retriaged)

	h.clock.t = h.clock.t.Add(3 * time.Minute) // t+5m
	assigned, err := h.trans.Assign(context.Background(), ws, id, "alice", "alice")
	require.NoError(t, err)
	require.Equal(t, domain.StatusAssigned, assigned.Status())
	require.Equal(t, []time.Duration{5 * time.Minute}, h.mx.mtta)

	_, err = h.trans.StartMitigation(context.Background(), ws, id, "alice")
	require.NoError(t, err)

	h.clock.t = h.clock.t.Add(25 * time.Minute) // t+30m
	_, err = h.trans.Resolve(context.Background(), ws, id, "fixed", "alice")
	require.NoError(t, err)
	require.Equal(t, []time.Duration{30 * time.Minute}, h.mx.mttr)

	pmp, err := h.trans.RequirePostmortem(context.Background(), ws, id, "system")
	require.NoError(t, err)
	require.Equal(t, domain.StatusPostmortemPending, pmp.Status())

	closed, err := h.trans.Close(context.Background(), ws, id, "done", "alice")
	require.NoError(t, err)
	require.Equal(t, domain.StatusClosed, closed.Status())

	require.Contains(t, h.pub.events, domain.EventIncidentResolved)
	require.Contains(t, h.pub.events, domain.EventIncidentClosed)
}

func TestTransitionWrongWorkspace(t *testing.T) {
	h := newHarness()
	inc := h.createOpen(t)
	_, err := h.trans.Triage(context.Background(), "other-ws", inc.ID().String(), "SEV1", "x", "mallory")
	require.ErrorIs(t, err, domain.ErrIncidentNotFound)
}

func TestInvalidTransitionPropagates(t *testing.T) {
	h := newHarness()
	inc := h.createOpen(t)
	// Assign trực tiếp từ Open (chưa Triage) → ErrInvalidTransition.
	_, err := h.trans.Assign(context.Background(), "ws-1", inc.ID().String(), "alice", "alice")
	require.ErrorIs(t, err, domain.ErrInvalidTransition)
}
