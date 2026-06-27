package command_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/notification/app/command"
	"github.com/namdam97/logmon/backend/internal/notification/domain"
)

type fakeTx struct{}

func (fakeTx) WithinTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type fakeRepo struct {
	saved     *domain.Channel
	updated   *domain.Channel
	deletedID string
	exists    bool
	existsErr error
	saveErr   error
	deleteErr error
}

func (r *fakeRepo) Save(_ context.Context, c domain.Channel) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	r.saved = &c
	return nil
}
func (r *fakeRepo) Update(_ context.Context, c domain.Channel) error { r.updated = &c; return nil }
func (r *fakeRepo) Delete(_ context.Context, _ string, id domain.ChannelID) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	r.deletedID = id.String()
	return nil
}
func (r *fakeRepo) ExistsByName(context.Context, string, string) (bool, error) {
	return r.exists, r.existsErr
}

type fakeReader struct {
	channel domain.Channel
	err     error
}

func (r *fakeReader) ByID(context.Context, string, domain.ChannelID) (domain.Channel, error) {
	return r.channel, r.err
}
func (r *fakeReader) List(context.Context, string) ([]domain.Channel, error) { return nil, nil }
func (r *fakeReader) SubscribedTo(context.Context, string, string) ([]domain.Channel, error) {
	return nil, nil
}

type fakeIDs struct{ id string }

func (g fakeIDs) NewID() string { return g.id }

type fakeClock struct{ t time.Time }

func (c fakeClock) Now() time.Time { return c.t }

func validInput() command.CreateChannelInput {
	return command.CreateChannelInput{
		WorkspaceID: "ws-1",
		Name:        "team slack",
		ChannelType: "slack",
		Config:      map[string]string{"webhook_url": "https://hooks.slack.com/x"},
		Events:      []string{domain.EventAlertFired},
	}
}

func TestCreateChannelSuccess(t *testing.T) {
	repo := &fakeRepo{}
	h := command.NewCreateChannelHandler(fakeTx{}, repo, fakeIDs{id: "11111111-1111-1111-1111-111111111111"}, fakeClock{t: time.Now()})

	got, err := h.Handle(context.Background(), validInput())

	require.NoError(t, err)
	require.Equal(t, "team slack", got.Name())
	require.NotNil(t, repo.saved)
	require.Equal(t, "11111111-1111-1111-1111-111111111111", got.ID().String())
}

func TestCreateChannelNameTaken(t *testing.T) {
	repo := &fakeRepo{exists: true}
	h := command.NewCreateChannelHandler(fakeTx{}, repo, fakeIDs{id: "x"}, fakeClock{t: time.Now()})

	_, err := h.Handle(context.Background(), validInput())

	require.ErrorIs(t, err, domain.ErrChannelNameTaken)
	require.Nil(t, repo.saved)
}

func TestCreateChannelInvalidType(t *testing.T) {
	h := command.NewCreateChannelHandler(fakeTx{}, &fakeRepo{}, fakeIDs{id: "x"}, fakeClock{t: time.Now()})

	in := validInput()
	in.ChannelType = "sms"
	_, err := h.Handle(context.Background(), in)

	var ve *domain.ValidationError
	require.ErrorAs(t, err, &ve)
	require.Equal(t, "channelType", ve.Field)
}

func TestUpdateChannelSuccess(t *testing.T) {
	now := time.Date(2026, 6, 27, 0, 0, 0, 0, time.UTC)
	id, _ := domain.NewChannelID("ch-1")
	existing, err := domain.NewChannel(domain.NewChannelInput{
		ID: id, WorkspaceID: "ws-1", Name: "old", ChannelType: domain.ChannelSlack,
		Config: map[string]string{"webhook_url": "https://hooks.slack.com/x"},
		Events: []string{domain.EventAlertFired}, CreatedAt: now,
	})
	require.NoError(t, err)
	repo := &fakeRepo{}
	reader := &fakeReader{channel: existing}
	h := command.NewUpdateChannelHandler(fakeTx{}, repo, reader, fakeClock{t: now.Add(time.Hour)})

	got, err := h.Handle(context.Background(), command.UpdateChannelInput{
		WorkspaceID: "ws-1", ID: "ch-1", Name: "new", ChannelType: "slack",
		Config: map[string]string{"webhook_url": "https://hooks.slack.com/y"},
		Events: []string{domain.EventAlertResolved}, Enabled: true,
	})

	require.NoError(t, err)
	require.Equal(t, "new", got.Name())
	require.NotNil(t, repo.updated)
}

func TestUpdateChannelNotFound(t *testing.T) {
	reader := &fakeReader{err: domain.ErrChannelNotFound}
	h := command.NewUpdateChannelHandler(fakeTx{}, &fakeRepo{}, reader, fakeClock{t: time.Now()})

	_, err := h.Handle(context.Background(), command.UpdateChannelInput{
		WorkspaceID: "ws-1", ID: "ch-x", Name: "n", ChannelType: "slack",
		Config: map[string]string{"webhook_url": "u"}, Events: []string{domain.EventAlertFired},
	})

	require.ErrorIs(t, err, domain.ErrChannelNotFound)
}

func TestDeleteChannel(t *testing.T) {
	repo := &fakeRepo{}
	h := command.NewDeleteChannelHandler(fakeTx{}, repo)

	err := h.Handle(context.Background(), "ws-1", "ch-1")

	require.NoError(t, err)
	require.Equal(t, "ch-1", repo.deletedID)
}

func TestDeleteChannelError(t *testing.T) {
	repo := &fakeRepo{deleteErr: errors.New("boom")}
	h := command.NewDeleteChannelHandler(fakeTx{}, repo)

	err := h.Handle(context.Background(), "ws-1", "ch-1")

	require.Error(t, err)
}
