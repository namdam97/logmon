package command_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/incident/app/command"
	"github.com/namdam97/logmon/backend/internal/incident/domain"
)

// ---- fakes postmortem ----

type fakePostmortemStore struct{ byIncident map[string]domain.Postmortem }

func newFakePostmortemStore() *fakePostmortemStore {
	return &fakePostmortemStore{byIncident: map[string]domain.Postmortem{}}
}

func (s *fakePostmortemStore) Save(_ context.Context, pm domain.Postmortem) error {
	s.byIncident[pm.IncidentID().String()] = pm
	return nil
}

func (s *fakePostmortemStore) Update(_ context.Context, pm domain.Postmortem) error {
	s.byIncident[pm.IncidentID().String()] = pm
	return nil
}

func (s *fakePostmortemStore) ByIncident(_ context.Context, id domain.IncidentID) (domain.Postmortem, error) {
	pm, ok := s.byIncident[id.String()]
	if !ok {
		return domain.Postmortem{}, domain.ErrPostmortemNotFound
	}
	return pm, nil
}

type fakeActionStore struct{ items map[string]domain.ActionItem }

func newFakeActionStore() *fakeActionStore {
	return &fakeActionStore{items: map[string]domain.ActionItem{}}
}

func (s *fakeActionStore) Save(_ context.Context, item domain.ActionItem) error {
	s.items[item.ID()] = item
	return nil
}

func (s *fakeActionStore) Update(_ context.Context, item domain.ActionItem) error {
	s.items[item.ID()] = item
	return nil
}

func (s *fakeActionStore) ByID(_ context.Context, id string) (domain.ActionItem, error) {
	item, ok := s.items[id]
	if !ok {
		return domain.ActionItem{}, domain.ErrActionItemNotFound
	}
	return item, nil
}

func (s *fakeActionStore) ListByPostmortem(_ context.Context, pid domain.PostmortemID) ([]domain.ActionItem, error) {
	var out []domain.ActionItem
	for _, item := range s.items {
		if item.PostmortemID() == pid {
			out = append(out, item)
		}
	}
	return out, nil
}

type fakeDueReader struct{ items []domain.Incident }

func (r *fakeDueReader) ListResolvedNeedingPostmortem(_ context.Context, _ time.Time) ([]domain.Incident, error) {
	return r.items, nil
}

type stubTransitioner struct {
	calls   int
	errFor  map[string]error // incidentID → err
	wsCalls []string
}

func (s *stubTransitioner) RequirePostmortem(_ context.Context, ws, id, _ string) (domain.Incident, error) {
	s.calls++
	s.wsCalls = append(s.wsCalls, ws)
	if s.errFor != nil {
		if err, ok := s.errFor[id]; ok {
			return domain.Incident{}, err
		}
	}
	return domain.Incident{}, nil
}

// pmHarness gom handler postmortem + store.
type pmHarness struct {
	h     *command.PostmortemHandler
	repo  *fakeRepo
	pmSt  *fakePostmortemStore
	actSt *fakeActionStore
	clock *fixedClock
}

func newPMHarness(t *testing.T) *pmHarness {
	t.Helper()
	repo := newFakeRepo()
	require.NoError(t, repo.Save(context.Background(), mkIncident(t, "inc-1", "ws-1", time.Date(2026, 6, 27, 10, 0, 0, 0, time.UTC))))
	pmSt := newFakePostmortemStore()
	actSt := newFakeActionStore()
	clock := &fixedClock{t: time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)}
	h := command.NewPostmortemHandler(repo, pmSt, pmSt, actSt, actSt, &seqIDs{}, clock)
	return &pmHarness{h: h, repo: repo, pmSt: pmSt, actSt: actSt, clock: clock}
}

func TestSubmitPostmortemCreateThenUpdate(t *testing.T) {
	h := newPMHarness(t)
	pm, err := h.h.Submit(context.Background(), command.SubmitPostmortemInput{
		WorkspaceID: "ws-1", IncidentID: "inc-1", RootCause: "pool exhausted",
	})
	require.NoError(t, err)
	require.Equal(t, domain.PostmortemDraft, pm.Status())
	require.Len(t, h.pmSt.byIncident, 1)

	updated, err := h.h.Submit(context.Background(), command.SubmitPostmortemInput{
		WorkspaceID: "ws-1", IncidentID: "inc-1", RootCause: "pool exhausted v2",
	})
	require.NoError(t, err)
	require.Equal(t, pm.ID().String(), updated.ID().String(), "cùng postmortem, không tạo mới")
	require.Equal(t, "pool exhausted v2", updated.RootCause())
	require.Len(t, h.pmSt.byIncident, 1)
}

func TestSubmitPostmortemWrongWorkspace(t *testing.T) {
	h := newPMHarness(t)
	_, err := h.h.Submit(context.Background(), command.SubmitPostmortemInput{
		WorkspaceID: "other-ws", IncidentID: "inc-1", RootCause: "x",
	})
	require.ErrorIs(t, err, domain.ErrIncidentNotFound)
}

func TestPublishPostmortem(t *testing.T) {
	h := newPMHarness(t)
	// Chưa đủ nội dung → publish lỗi.
	_, err := h.h.Submit(context.Background(), command.SubmitPostmortemInput{
		WorkspaceID: "ws-1", IncidentID: "inc-1", RootCause: "root only",
	})
	require.NoError(t, err)
	_, err = h.h.Publish(context.Background(), "ws-1", "inc-1")
	require.Error(t, err, "thiếu lessons learned")

	// Đủ nội dung → publish OK.
	_, err = h.h.Submit(context.Background(), command.SubmitPostmortemInput{
		WorkspaceID: "ws-1", IncidentID: "inc-1", RootCause: "root", LessonsLearned: "lessons",
	})
	require.NoError(t, err)
	pub, err := h.h.Publish(context.Background(), "ws-1", "inc-1")
	require.NoError(t, err)
	require.Equal(t, domain.PostmortemPublished, pub.Status())
}

func TestActionItemFlow(t *testing.T) {
	h := newPMHarness(t)
	_, err := h.h.Submit(context.Background(), command.SubmitPostmortemInput{
		WorkspaceID: "ws-1", IncidentID: "inc-1", RootCause: "root",
	})
	require.NoError(t, err)

	item, err := h.h.AddActionItem(context.Background(), command.AddActionItemInput{
		WorkspaceID: "ws-1", IncidentID: "inc-1", Title: "Add alert", Assignee: "alice",
	})
	require.NoError(t, err)
	require.Equal(t, domain.ActionOpen, item.Status())

	done, err := h.h.UpdateActionItemStatus(context.Background(), "ws-1", "inc-1", item.ID(), "done")
	require.NoError(t, err)
	require.Equal(t, domain.ActionDone, done.Status())
	require.NotNil(t, done.CompletedAt())
}

func TestAddActionItemNoPostmortem(t *testing.T) {
	h := newPMHarness(t)
	_, err := h.h.AddActionItem(context.Background(), command.AddActionItemInput{
		WorkspaceID: "ws-1", IncidentID: "inc-1", Title: "x",
	})
	require.ErrorIs(t, err, domain.ErrPostmortemNotFound)
}

func TestPostmortemReminderSweep(t *testing.T) {
	due := &fakeDueReader{items: []domain.Incident{
		mkIncident(t, "inc-1", "ws-1", time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC)),
		mkIncident(t, "inc-2", "ws-2", time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC)),
	}}
	trans := &stubTransitioner{}
	svc := command.NewPostmortemReminderService(due, trans, &fixedClock{t: time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)}, 0)

	flagged, err := svc.Sweep(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, flagged)
	require.Equal(t, 2, trans.calls)
	require.ElementsMatch(t, []string{"ws-1", "ws-2"}, trans.wsCalls)
}

func TestPostmortemReminderIgnoresInvalidTransition(t *testing.T) {
	due := &fakeDueReader{items: []domain.Incident{
		mkIncident(t, "inc-1", "ws-1", time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC)),
		mkIncident(t, "inc-2", "ws-1", time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC)),
	}}
	trans := &stubTransitioner{errFor: map[string]error{"inc-2": domain.ErrInvalidTransition}}
	svc := command.NewPostmortemReminderService(due, trans, &fixedClock{t: time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)}, 24*time.Hour)

	flagged, err := svc.Sweep(context.Background())
	require.NoError(t, err, "ErrInvalidTransition (race) không tính là lỗi")
	require.Equal(t, 1, flagged)
}
