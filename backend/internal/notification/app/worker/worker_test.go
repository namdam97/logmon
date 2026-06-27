package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/notification/domain"
	"github.com/namdam97/logmon/backend/internal/notification/ports"
)

type fakeSender struct {
	err   error
	calls int
}

func (s *fakeSender) Send(context.Context, domain.Message) error {
	s.calls++
	return s.err
}

type fakeEnq struct{ items []domain.Message }

func (e *fakeEnq) Enqueue(_ context.Context, msg domain.Message, _ time.Duration) error {
	e.items = append(e.items, msg)
	return nil
}

type fakeHistory struct{ entries []domain.HistoryEntry }

func (h *fakeHistory) Save(_ context.Context, e domain.HistoryEntry) error {
	h.entries = append(h.entries, e)
	return nil
}

type fakeQueue struct{ acked []string }

func (q *fakeQueue) Read(context.Context, int, time.Duration) ([]ports.QueueItem, error) {
	return nil, nil
}
func (q *fakeQueue) Ack(_ context.Context, ids ...string) error {
	q.acked = append(q.acked, ids...)
	return nil
}

type fakeClock struct{}

func (fakeClock) Now() time.Time { return time.Unix(0, 0) }

func newTestWorker(senders map[string]ports.Sender, enq *fakeEnq, hist *fakeHistory, q *fakeQueue) *Worker {
	return NewWorker(q, enq, senders, hist, fakeClock{}, nil)
}

func msg() domain.Message {
	return domain.Message{ChannelID: "ch-1", WorkspaceID: "ws-1", ChannelType: "slack", EventType: domain.EventAlertFired, EventRef: "a-1"}
}

func TestProcessSentRecordsHistoryAndAcks(t *testing.T) {
	sender := &fakeSender{}
	enq := &fakeEnq{}
	hist := &fakeHistory{}
	q := &fakeQueue{}
	w := newTestWorker(map[string]ports.Sender{"slack": sender}, enq, hist, q)

	w.process(context.Background(), ports.QueueItem{ID: "1", Msg: msg()})

	require.Equal(t, 1, sender.calls)
	require.Len(t, hist.entries, 1)
	require.Equal(t, domain.StatusSent, hist.entries[0].Status)
	require.Equal(t, []string{"1"}, q.acked)
	require.Empty(t, enq.items, "no retry on success")
}

func TestProcessFailureRetriesWithBackoff(t *testing.T) {
	sender := &fakeSender{err: errors.New("503")}
	enq := &fakeEnq{}
	hist := &fakeHistory{}
	q := &fakeQueue{}
	w := newTestWorker(map[string]ports.Sender{"slack": sender}, enq, hist, q)

	w.process(context.Background(), ports.QueueItem{ID: "1", Msg: msg()})

	require.Len(t, enq.items, 1, "re-enqueued for retry")
	require.Equal(t, 1, enq.items[0].Attempt)
	require.Equal(t, domain.StatusRetrying, hist.entries[0].Status)
	require.Equal(t, []string{"1"}, q.acked)
}

func TestProcessGivesUpAfterMaxAttempts(t *testing.T) {
	sender := &fakeSender{err: errors.New("503")}
	enq := &fakeEnq{}
	hist := &fakeHistory{}
	q := &fakeQueue{}
	w := newTestWorker(map[string]ports.Sender{"slack": sender}, enq, hist, q)

	m := msg()
	m.Attempt = len(_retryBackoff) - 1 // last attempt
	w.process(context.Background(), ports.QueueItem{ID: "1", Msg: m})

	require.Empty(t, enq.items, "no further retry")
	require.Equal(t, domain.StatusFailed, hist.entries[0].Status)
}

func TestProcessUnsupportedChannelTypeFails(t *testing.T) {
	enq := &fakeEnq{}
	hist := &fakeHistory{}
	q := &fakeQueue{}
	w := newTestWorker(map[string]ports.Sender{}, enq, hist, q)

	w.process(context.Background(), ports.QueueItem{ID: "1", Msg: msg()})

	require.Equal(t, domain.StatusFailed, hist.entries[0].Status)
	require.Contains(t, hist.entries[0].ErrorMessage, "unsupported")
	require.Equal(t, []string{"1"}, q.acked)
}

func TestProcessOpenBreakerDefersWithoutSending(t *testing.T) {
	sender := &fakeSender{err: errors.New("down")}
	enq := &fakeEnq{}
	hist := &fakeHistory{}
	q := &fakeQueue{}
	w := newTestWorker(map[string]ports.Sender{"slack": sender}, enq, hist, q)

	// Trip the breaker (threshold = 5 consecutive failures). Each failure also
	// re-enqueues; we only assert the breaker eventually blocks the send.
	for i := 0; i < _breakerThreshold; i++ {
		w.process(context.Background(), ports.QueueItem{ID: "x", Msg: msg()})
	}
	callsBefore := sender.calls

	w.process(context.Background(), ports.QueueItem{ID: "y", Msg: msg()})

	require.Equal(t, callsBefore, sender.calls, "send skipped while breaker open")
	require.NotEmpty(t, enq.items, "deferred via re-enqueue")
}
