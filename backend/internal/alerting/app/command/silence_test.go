package command_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/alerting/app/command"
	"github.com/namdam97/logmon/backend/internal/alerting/domain"
)

// fakeSilenceGateway ghi lại silence được tạo + cho phép giả lập lỗi.
type fakeSilenceGateway struct {
	created   domain.Silence
	createID  string
	createErr error
	deletedID string
	deleteErr error
}

func (f *fakeSilenceGateway) Create(_ context.Context, s domain.Silence) (string, error) {
	f.created = s
	return f.createID, f.createErr
}

func (f *fakeSilenceGateway) Delete(_ context.Context, id string) error {
	f.deletedID = id
	return f.deleteErr
}

func (f *fakeSilenceGateway) List(context.Context) ([]domain.SilenceView, error) {
	return nil, nil
}

func validCreateSilenceInput() command.CreateSilenceInput {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return command.CreateSilenceInput{
		Matchers:  []domain.MatcherInput{{Name: "alertname", Value: "HighErrorRate", IsEqual: true}},
		StartsAt:  start,
		EndsAt:    start.Add(2 * time.Hour),
		CreatedBy: "user-1",
		Comment:   "điều tra",
	}
}

func TestCreateSilence_Success(t *testing.T) {
	gw := &fakeSilenceGateway{createID: "sil-123"}
	h := command.NewCreateSilenceHandler(gw, fixedClock{})

	id, err := h.Handle(context.Background(), validCreateSilenceInput())

	require.NoError(t, err)
	require.Equal(t, "sil-123", id)
	require.Equal(t, "user-1", gw.created.CreatedBy())
	require.Len(t, gw.created.Matchers(), 1)
}

func TestCreateSilence_DefaultsStartsAtToNow(t *testing.T) {
	gw := &fakeSilenceGateway{createID: "sil-1"}
	h := command.NewCreateSilenceHandler(gw, fixedClock{})

	in := validCreateSilenceInput()
	in.StartsAt = time.Time{} // rỗng → mặc định clock.Now()
	_, err := h.Handle(context.Background(), in)

	require.NoError(t, err)
	require.Equal(t, fixedClock{}.Now(), gw.created.StartsAt())
}

func TestCreateSilence_ValidationErrorNotProxied(t *testing.T) {
	gw := &fakeSilenceGateway{}
	h := command.NewCreateSilenceHandler(gw, fixedClock{})

	in := validCreateSilenceInput()
	in.Matchers = nil // không hợp lệ
	_, err := h.Handle(context.Background(), in)

	var ve *domain.ValidationError
	require.True(t, errors.As(err, &ve))
	require.Equal(t, domain.Silence{}, gw.created, "không gọi gateway khi input không hợp lệ")
}

func TestDeleteSilence_DelegatesToGateway(t *testing.T) {
	gw := &fakeSilenceGateway{}
	h := command.NewDeleteSilenceHandler(gw)

	require.NoError(t, h.Handle(context.Background(), "sil-123"))
	require.Equal(t, "sil-123", gw.deletedID)
}

func TestDeleteSilence_PropagatesNotFound(t *testing.T) {
	gw := &fakeSilenceGateway{deleteErr: domain.ErrSilenceNotFound}
	h := command.NewDeleteSilenceHandler(gw)

	err := h.Handle(context.Background(), "nope")

	require.True(t, errors.Is(err, domain.ErrSilenceNotFound))
}
