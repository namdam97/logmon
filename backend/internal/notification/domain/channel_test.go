package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/notification/domain"
)

func mustID(t *testing.T) domain.ChannelID {
	t.Helper()
	id, err := domain.NewChannelID("ch-1")
	require.NoError(t, err)
	return id
}

func validSlack(t *testing.T) domain.NewChannelInput {
	t.Helper()
	return domain.NewChannelInput{
		ID:          mustID(t),
		WorkspaceID: "ws-1",
		Name:        "team slack",
		ChannelType: domain.ChannelSlack,
		Config:      map[string]string{"webhook_url": "https://hooks.slack.com/x"},
		Events:      []string{domain.EventAlertFired, domain.EventIncidentCreated},
		CreatedAt:   time.Date(2026, 6, 27, 0, 0, 0, 0, time.UTC),
	}
}

func TestNewChannelSlack(t *testing.T) {
	got, err := domain.NewChannel(validSlack(t))

	require.NoError(t, err)
	require.Equal(t, domain.ChannelSlack, got.Type())
	require.True(t, got.IsEnabled())
	require.True(t, got.SubscribesTo(domain.EventAlertFired))
	require.False(t, got.SubscribesTo(domain.EventSLOBudgetWarning))
	require.Equal(t, "https://hooks.slack.com/x", got.ConfigValue("webhook_url"))
}

func TestNewChannelType(t *testing.T) {
	_, err := domain.NewChannelType("sms")
	requireValidation(t, err, "channelType")

	got, err := domain.NewChannelType("pagerduty")
	require.NoError(t, err)
	require.Equal(t, domain.ChannelPagerDuty, got)
}

func TestNewChannelValidation(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(in *domain.NewChannelInput)
		wantField string
	}{
		{"empty name", func(in *domain.NewChannelInput) { in.Name = " " }, "name"},
		{"missing config key", func(in *domain.NewChannelInput) { in.Config = map[string]string{} }, "config.webhook_url"},
		{"no events", func(in *domain.NewChannelInput) { in.Events = nil }, "events"},
		{"bad event type", func(in *domain.NewChannelInput) { in.Events = []string{"Bad Event"} }, "events"},
		{"empty workspace", func(in *domain.NewChannelInput) { in.WorkspaceID = "" }, "workspaceId"},
		{"zero createdAt", func(in *domain.NewChannelInput) { in.CreatedAt = time.Time{} }, "createdAt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validSlack(t)
			tt.mutate(&in)
			_, err := domain.NewChannel(in)
			requireValidation(t, err, tt.wantField)
		})
	}
}

func TestChannelUpdateImmutable(t *testing.T) {
	c, err := domain.NewChannel(validSlack(t))
	require.NoError(t, err)

	now := time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC)
	updated, err := c.Update(domain.UpdateInput{
		Name:        "renamed",
		ChannelType: domain.ChannelSlack,
		Config:      map[string]string{"webhook_url": "https://hooks.slack.com/y"},
		Events:      []string{domain.EventAlertResolved},
		Enabled:     false,
	}, now)

	require.NoError(t, err)
	require.Equal(t, "renamed", updated.Name())
	require.False(t, updated.IsEnabled())
	require.True(t, updated.SubscribesTo(domain.EventAlertResolved))
	// gốc không đổi
	require.Equal(t, "team slack", c.Name())
	require.True(t, c.IsEnabled())
}

func TestChannelConfigCopyIsolation(t *testing.T) {
	c, err := domain.NewChannel(validSlack(t))
	require.NoError(t, err)
	cfg := c.Config()
	cfg["webhook_url"] = "MUTATED"
	require.Equal(t, "https://hooks.slack.com/x", c.ConfigValue("webhook_url"))
}

func requireValidation(t *testing.T, err error, field string) {
	t.Helper()
	require.Error(t, err)
	var ve *domain.ValidationError
	require.True(t, errors.As(err, &ve), "expected ValidationError, got %v", err)
	require.Equal(t, field, ve.Field)
}
