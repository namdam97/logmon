package command_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/logpipeline/app/command"
	"github.com/namdam97/logmon/backend/internal/logpipeline/domain"
)

// ---- fakes ----

type fakeConfigStore struct {
	cfg     *domain.PipelineConfig
	upserts int
}

func (s *fakeConfigStore) Get(_ context.Context, _ string) (domain.PipelineConfig, error) {
	if s.cfg == nil {
		return domain.PipelineConfig{}, domain.ErrPipelineConfigNotFound
	}
	return *s.cfg, nil
}
func (s *fakeConfigStore) Upsert(_ context.Context, c domain.PipelineConfig) error {
	s.upserts++
	cp := c
	s.cfg = &cp
	return nil
}

type fakeILM struct {
	applied *domain.ILMPolicy
	err     error
}

func (f *fakeILM) Apply(_ context.Context, _ string, p domain.ILMPolicy) error {
	if f.err != nil {
		return f.err
	}
	f.applied = &p
	return nil
}

type fakeDLQStore struct {
	entries map[int64]domain.DLQEntry
	updated map[int64]domain.DLQStatus
}

func newFakeDLQStore() *fakeDLQStore {
	return &fakeDLQStore{entries: map[int64]domain.DLQEntry{}, updated: map[int64]domain.DLQStatus{}}
}

func (s *fakeDLQStore) ByID(_ context.Context, _ string, id int64) (domain.DLQEntry, error) {
	e, ok := s.entries[id]
	if !ok {
		return domain.DLQEntry{}, domain.ErrDLQEntryNotFound
	}
	return e, nil
}
func (s *fakeDLQStore) UpdateStatus(_ context.Context, _ string, id int64, st domain.DLQStatus, _ int, _ *time.Time) error {
	s.updated[id] = st
	return nil
}

type fakeReplayer struct {
	replayed []int64
	err      error
}

func (f *fakeReplayer) Replay(_ context.Context, e domain.DLQEntry) error {
	if f.err != nil {
		return f.err
	}
	f.replayed = append(f.replayed, e.ID())
	return nil
}

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// ---- tests ----

func TestSwitchMode(t *testing.T) {
	store := &fakeConfigStore{}
	cmd := command.NewPipelineCommands(store, nil, fixedClock{t: time.Unix(10, 0).UTC()})

	cfg, err := cmd.SwitchMode(context.Background(), "ws-1", "B", "u-1")
	require.NoError(t, err)
	require.Equal(t, domain.ModeB, cfg.Mode())
	require.Equal(t, "u-1", cfg.UpdatedBy())
	require.Equal(t, 1, store.upserts)

	_, err = cmd.SwitchMode(context.Background(), "ws-1", "Z", "u-1")
	require.Error(t, err)
}

func TestUpdateILMAppliesThenPersists(t *testing.T) {
	store := &fakeConfigStore{}
	ilm := &fakeILM{}
	cmd := command.NewPipelineCommands(store, ilm, fixedClock{t: time.Unix(10, 0).UTC()})

	cfg, err := cmd.UpdateILM(context.Background(), "ws-1", "default",
		command.UpdateILMInput{HotDays: 3, WarmDays: 10, DeleteDays: 60}, "u-1")
	require.NoError(t, err)
	require.Equal(t, 3, cfg.ILM().HotDays)
	require.NotNil(t, ilm.applied)
	require.Equal(t, 1, store.upserts)
}

func TestUpdateILMRejectsInvalid(t *testing.T) {
	store := &fakeConfigStore{}
	ilm := &fakeILM{}
	cmd := command.NewPipelineCommands(store, ilm, fixedClock{t: time.Unix(10, 0).UTC()})
	_, err := cmd.UpdateILM(context.Background(), "ws-1", "default",
		command.UpdateILMInput{HotDays: 30, WarmDays: 10, DeleteDays: 60}, "u-1")
	require.Error(t, err)
	require.Nil(t, ilm.applied) // không áp ES khi validate fail
	require.Equal(t, 0, store.upserts)
}

func TestUpdateILMNotPersistedWhenApplyFails(t *testing.T) {
	store := &fakeConfigStore{}
	ilm := &fakeILM{err: errors.New("es down")}
	cmd := command.NewPipelineCommands(store, ilm, fixedClock{t: time.Unix(10, 0).UTC()})
	_, err := cmd.UpdateILM(context.Background(), "ws-1", "default",
		command.UpdateILMInput{HotDays: 3, WarmDays: 10, DeleteDays: 60}, "u-1")
	require.Error(t, err)
	require.Equal(t, 0, store.upserts) // không persist khi ES từ chối
}

func TestDLQRetry(t *testing.T) {
	store := newFakeDLQStore()
	now := time.Unix(100, 0).UTC()
	store.entries[1] = domain.ReconstructDLQEntry(1, "ws-1", "raw", "reject", "svc", 0, domain.DLQPending, now, nil)
	store.entries[2] = domain.ReconstructDLQEntry(2, "ws-1", "raw", "reject", "svc", 1, domain.DLQRetried, now, &now)

	replayer := &fakeReplayer{}
	cmd := command.NewDLQCommands(store, replayer, fixedClock{t: now})

	res, err := cmd.Retry(context.Background(), "ws-1", []int64{1, 2, 99})
	require.NoError(t, err)
	require.Equal(t, []int64{1}, res.Retried)             // chỉ entry 1 pending
	require.Contains(t, res.Failed, int64(2))             // đã retried → not retryable
	require.Contains(t, res.Failed, int64(99))            // không tồn tại
	require.Equal(t, []int64{1}, replayer.replayed)       // chỉ replay entry hợp lệ
	require.Equal(t, domain.DLQRetried, store.updated[1]) // persist trạng thái
}
