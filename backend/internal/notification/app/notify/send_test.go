package notify

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/notification/domain"
)

type fakeReader struct {
	channels []domain.Channel
	err      error
}

func (f *fakeReader) ByID(context.Context, string, domain.ChannelID) (domain.Channel, error) {
	return domain.Channel{}, nil
}
func (f *fakeReader) List(context.Context, string) ([]domain.Channel, error) { return nil, nil }
func (f *fakeReader) SubscribedTo(context.Context, string, string) ([]domain.Channel, error) {
	return f.channels, f.err
}

type fakeQueue struct {
	enqueued []domain.Message
	err      error
}

func (f *fakeQueue) Enqueue(_ context.Context, msg domain.Message, _ time.Duration) error {
	if f.err != nil {
		return f.err
	}
	f.enqueued = append(f.enqueued, msg)
	return nil
}

type fakeLogger struct{ warned []string }

func (f *fakeLogger) Warn(_ context.Context, msg string) { f.warned = append(f.warned, msg) }

func slackChannel(t *testing.T, name string, events ...string) domain.Channel {
	t.Helper()
	id, err := domain.NewChannelID("ch-" + name)
	require.NoError(t, err)
	c, err := domain.NewChannel(domain.NewChannelInput{
		ID:          id,
		WorkspaceID: "ws-1",
		Name:        name,
		ChannelType: domain.ChannelSlack,
		Config:      map[string]string{"webhook_url": "https://hooks.slack.com/x"},
		Events:      events,
		CreatedAt:   time.Date(2026, 6, 27, 0, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	return c
}

func TestSendEnqueuesRenderedMessagePerChannel(t *testing.T) {
	reader := &fakeReader{channels: []domain.Channel{
		slackChannel(t, "a", domain.EventAlertFired),
		slackChannel(t, "b", domain.EventAlertFired),
	}}
	queue := &fakeQueue{}
	h, err := NewSendHandler(reader, queue, &fakeLogger{})
	require.NoError(t, err)

	err = h.Handle(context.Background(), SendInput{
		WorkspaceID: "ws-1",
		EventType:   domain.EventAlertFired,
		EventRef:    "alert-7",
		Data:        map[string]string{"alertName": "HighErrorRate", "severity": "critical", "service": "userservice"},
		DedupKey:    "alert-7",
	})

	require.NoError(t, err)
	require.Len(t, queue.enqueued, 2)
	got := queue.enqueued[0]
	require.Equal(t, "slack", got.ChannelType)
	require.Equal(t, "alert-7", got.EventRef)
	require.Contains(t, got.Subject, "HighErrorRate")
	require.Contains(t, got.Subject, "critical")
	require.Contains(t, got.Body, "userservice")
	require.Equal(t, "https://hooks.slack.com/x", got.Config["webhook_url"])
}

func TestSendNoChannelIsNoOp(t *testing.T) {
	queue := &fakeQueue{}
	log := &fakeLogger{}
	h, err := NewSendHandler(&fakeReader{channels: nil}, queue, log)
	require.NoError(t, err)

	err = h.Handle(context.Background(), SendInput{WorkspaceID: "ws-1", EventType: domain.EventAlertFired})

	require.NoError(t, err)
	require.Empty(t, queue.enqueued)
	require.Len(t, log.warned, 1)
}

func TestSendReturnsReaderError(t *testing.T) {
	h, err := NewSendHandler(&fakeReader{err: errors.New("db down")}, &fakeQueue{}, &fakeLogger{})
	require.NoError(t, err)

	err = h.Handle(context.Background(), SendInput{WorkspaceID: "ws-1", EventType: domain.EventAlertFired})

	require.Error(t, err)
}

func TestSendContinuesOnEnqueueError(t *testing.T) {
	reader := &fakeReader{channels: []domain.Channel{slackChannel(t, "a", domain.EventAlertFired)}}
	h, err := NewSendHandler(reader, &fakeQueue{err: errors.New("redis down")}, &fakeLogger{})
	require.NoError(t, err)

	err = h.Handle(context.Background(), SendInput{WorkspaceID: "ws-1", EventType: domain.EventAlertFired})

	require.Error(t, err)
}

func TestRenderFallbackForUnknownEvent(t *testing.T) {
	r, err := newRenderer()
	require.NoError(t, err)

	subject, body := r.render("custom_event", map[string]string{"k": "v"})

	require.Contains(t, subject, "custom_event")
	require.Contains(t, body, "k: v")
}

func TestRenderAllKnownTemplatesParse(t *testing.T) {
	r, err := newRenderer()
	require.NoError(t, err)
	events := []string{
		domain.EventAlertFired, domain.EventAlertResolved,
		domain.EventIncidentCreated, domain.EventIncidentResolved,
		domain.EventSLOBudgetWarning,
	}
	for _, e := range events {
		subject, body := r.render(e, map[string]string{})
		require.NotEmpty(t, subject, e)
		require.NotEmpty(t, body, e)
	}
}
