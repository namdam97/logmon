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

const fixtureID = "11111111-1111-1111-1111-111111111111"

// seedRule nạp một rule hợp lệ (ws-default) vào repo để update/delete/toggle.
func seedRule(t *testing.T, repo *fakeRepo) domain.AlertRule {
	t.Helper()
	id, err := domain.NewRuleID(fixtureID)
	require.NoError(t, err)
	rule, err := domain.NewAlertRule(domain.NewAlertRuleInput{
		ID:          id,
		WorkspaceID: "ws-default",
		Name:        "HighErrorRate",
		Expression:  "up == 0",
		Service:     "logmon-api",
		ForDuration: 2 * time.Minute,
		Severity:    domain.SeverityCritical,
		Annotations: map[string]string{domain.AnnotationSummary: "s", domain.AnnotationRunbookURL: "u"},
		CreatedAt:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.NoError(t, repo.Save(context.Background(), rule))
	repo.saved = nil // reset để assertion update không lẫn với seed
	return rule
}

func validUpdate() command.UpdateRuleInput {
	return command.UpdateRuleInput{
		WorkspaceID: "ws-default",
		ID:          fixtureID,
		Name:        "HighErrorRate",
		Expression:  "up == 1",
		Service:     "logmon-api",
		Severity:    "critical",
		ForDuration: 3 * time.Minute,
		Annotations: map[string]string{domain.AnnotationSummary: "s2", domain.AnnotationRunbookURL: "u2"},
	}
}

func TestUpdateRule_Success(t *testing.T) {
	repo, pub := newFakeRepo(), &fakePublisher{}
	seedRule(t, repo)
	h := command.NewUpdateRuleHandler(fakeTx{}, repo, repo, pub, fakeValidator{}, fixedClock{})

	rule, err := h.Handle(context.Background(), validUpdate())

	require.NoError(t, err)
	require.Equal(t, "up == 1", rule.Expression())
	require.Equal(t, 3*time.Minute, rule.ForDuration())
	require.Equal(t, domain.SyncPending, rule.SyncStatus())
	require.Len(t, repo.updated, 1)
	require.Equal(t, []string{domain.EventAlertRuleUpdated}, pub.events)
}

func TestUpdateRule_NotFound(t *testing.T) {
	repo, pub := newFakeRepo(), &fakePublisher{}
	h := command.NewUpdateRuleHandler(fakeTx{}, repo, repo, pub, fakeValidator{}, fixedClock{})

	_, err := h.Handle(context.Background(), validUpdate())

	require.ErrorIs(t, err, domain.ErrRuleNotFound)
	require.Empty(t, repo.updated)
	require.Empty(t, pub.events)
}

func TestUpdateRule_WrongWorkspaceTreatedAsNotFound(t *testing.T) {
	repo, pub := newFakeRepo(), &fakePublisher{}
	seedRule(t, repo)
	in := validUpdate()
	in.WorkspaceID = "ws-other"
	h := command.NewUpdateRuleHandler(fakeTx{}, repo, repo, pub, fakeValidator{}, fixedClock{})

	_, err := h.Handle(context.Background(), in)

	require.ErrorIs(t, err, domain.ErrRuleNotFound)
	require.Empty(t, repo.updated)
}

func TestUpdateRule_RenameToTakenName(t *testing.T) {
	repo, pub := newFakeRepo(), &fakePublisher{}
	seedRule(t, repo)
	repo.names["ws-default|Other"] = true
	in := validUpdate()
	in.Name = "Other"
	h := command.NewUpdateRuleHandler(fakeTx{}, repo, repo, pub, fakeValidator{}, fixedClock{})

	_, err := h.Handle(context.Background(), in)

	require.ErrorIs(t, err, domain.ErrRuleNameTaken)
	require.Empty(t, repo.updated)
	require.Empty(t, pub.events)
}

func TestUpdateRule_InvalidPromQL(t *testing.T) {
	repo, pub := newFakeRepo(), &fakePublisher{}
	seedRule(t, repo)
	h := command.NewUpdateRuleHandler(fakeTx{}, repo, repo, pub, fakeValidator{err: errors.New("parse error")}, fixedClock{})

	_, err := h.Handle(context.Background(), validUpdate())

	var ve *domain.ValidationError
	require.True(t, errors.As(err, &ve))
	require.Equal(t, "expression", ve.Field)
	require.Empty(t, repo.updated)
}

func TestDeleteRule_Success(t *testing.T) {
	repo, pub := newFakeRepo(), &fakePublisher{}
	seedRule(t, repo)
	h := command.NewDeleteRuleHandler(fakeTx{}, repo, repo, pub)

	err := h.Handle(context.Background(), "ws-default", fixtureID)

	require.NoError(t, err)
	require.Equal(t, []string{fixtureID}, repo.deleted)
	require.Equal(t, []string{domain.EventAlertRuleDeleted}, pub.events)
}

func TestDeleteRule_NotFound(t *testing.T) {
	repo, pub := newFakeRepo(), &fakePublisher{}
	h := command.NewDeleteRuleHandler(fakeTx{}, repo, repo, pub)

	err := h.Handle(context.Background(), "ws-default", fixtureID)

	require.ErrorIs(t, err, domain.ErrRuleNotFound)
	require.Empty(t, repo.deleted)
	require.Empty(t, pub.events)
}

func TestSetRuleEnabled_Toggle(t *testing.T) {
	tests := []struct {
		name        string
		enabled     bool
		wantEnabled bool
	}{
		{name: "disable", enabled: false, wantEnabled: false},
		{name: "enable", enabled: true, wantEnabled: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, pub := newFakeRepo(), &fakePublisher{}
			seedRule(t, repo)
			h := command.NewSetRuleEnabledHandler(fakeTx{}, repo, repo, pub, fixedClock{})

			rule, err := h.Handle(context.Background(), "ws-default", fixtureID, tt.enabled)

			require.NoError(t, err)
			require.Equal(t, tt.wantEnabled, rule.IsEnabled())
			require.Equal(t, domain.SyncPending, rule.SyncStatus())
			require.Len(t, repo.updated, 1)
			require.Equal(t, []string{domain.EventAlertRuleUpdated}, pub.events)
		})
	}
}

func TestSetRuleEnabled_NotFound(t *testing.T) {
	repo, pub := newFakeRepo(), &fakePublisher{}
	h := command.NewSetRuleEnabledHandler(fakeTx{}, repo, repo, pub, fixedClock{})

	_, err := h.Handle(context.Background(), "ws-default", fixtureID, false)

	require.ErrorIs(t, err, domain.ErrRuleNotFound)
	require.Empty(t, repo.updated)
}
