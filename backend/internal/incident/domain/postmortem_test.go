package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/incident/domain"
)

func mkIncidentID(t *testing.T) domain.IncidentID {
	t.Helper()
	id, err := domain.NewIncidentID("inc-1")
	require.NoError(t, err)
	return id
}

func newDraft(t *testing.T) domain.Postmortem {
	t.Helper()
	pm, err := domain.NewPostmortem(domain.NewPostmortemInput{
		ID:          "pm-1",
		IncidentID:  mkIncidentID(t),
		WorkspaceID: "ws-1",
		Now:         time.Date(2026, 6, 27, 10, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	return pm
}

func TestNewPostmortem(t *testing.T) {
	pm := newDraft(t)
	require.Equal(t, domain.PostmortemDraft, pm.Status())
	require.Equal(t, "inc-1", pm.IncidentID().String())
	require.Nil(t, pm.PublishedAt())
}

func TestNewPostmortemValidation(t *testing.T) {
	base := domain.NewPostmortemInput{
		ID: "pm", IncidentID: mkIncidentID(t), WorkspaceID: "ws-1",
		Now: time.Date(2026, 6, 27, 10, 0, 0, 0, time.UTC),
	}
	tests := []struct {
		name    string
		mutate  func(in *domain.NewPostmortemInput)
		wantErr bool
	}{
		{name: "valid", mutate: func(*domain.NewPostmortemInput) {}},
		{name: "empty id", mutate: func(in *domain.NewPostmortemInput) { in.ID = "" }, wantErr: true},
		{name: "empty workspace", mutate: func(in *domain.NewPostmortemInput) { in.WorkspaceID = "" }, wantErr: true},
		{name: "negative duration", mutate: func(in *domain.NewPostmortemInput) { in.Impact = domain.Impact{DurationSeconds: -1} }, wantErr: true},
		{name: "negative error count", mutate: func(in *domain.NewPostmortemInput) { in.Impact = domain.Impact{ErrorCount: -5} }, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			give := base
			tt.mutate(&give)
			_, err := domain.NewPostmortem(give)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestPostmortemUpdateContent(t *testing.T) {
	pm := newDraft(t)
	updated, err := pm.UpdateContent(domain.UpdateContentInput{
		RootCause:      "DB connection pool exhausted",
		Impact:         domain.Impact{DurationSeconds: 1800, ErrorCount: 1200, BudgetConsumedPercent: 35.5},
		LessonsLearned: "Add pool size alert",
		Now:            time.Date(2026, 6, 27, 11, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.Equal(t, "DB connection pool exhausted", updated.RootCause())
	require.Equal(t, int64(1800), updated.Impact().DurationSeconds)
	// Bất biến: bản gốc không đổi.
	require.Empty(t, pm.RootCause())
}

func TestPostmortemPublishRequiresContent(t *testing.T) {
	pm := newDraft(t)
	_, err := pm.Publish(time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC))
	require.Error(t, err, "root cause rỗng không publish được")

	withRoot, err := pm.UpdateContent(domain.UpdateContentInput{
		RootCause: "root", Now: time.Date(2026, 6, 27, 11, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	_, err = withRoot.Publish(time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC))
	require.Error(t, err, "lessons learned rỗng không publish được")
}

func TestPostmortemPublishHappyPath(t *testing.T) {
	pm := newDraft(t)
	filled, err := pm.UpdateContent(domain.UpdateContentInput{
		RootCause: "root cause", LessonsLearned: "lessons",
		Now: time.Date(2026, 6, 27, 11, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	published, err := filled.Publish(time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Equal(t, domain.PostmortemPublished, published.Status())
	require.NotNil(t, published.PublishedAt())

	// Published là bất biến: không update, không publish lại.
	_, err = published.UpdateContent(domain.UpdateContentInput{RootCause: "x", Now: time.Now()})
	require.ErrorIs(t, err, domain.ErrPostmortemPublished)
	_, err = published.Publish(time.Now())
	require.ErrorIs(t, err, domain.ErrPostmortemPublished)
}

func TestNewActionItem(t *testing.T) {
	pid, err := domain.NewPostmortemID("pm-1")
	require.NoError(t, err)
	due := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	ai, err := domain.NewActionItem(domain.NewActionItemInput{
		ID: "ai-1", PostmortemID: pid, Title: "Add pool alert", Assignee: "alice",
		DueDate: &due, Now: time.Date(2026, 6, 27, 10, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.Equal(t, domain.ActionOpen, ai.Status())
	require.Equal(t, "alice", ai.Assignee())
	require.NotNil(t, ai.DueDate())
	require.Nil(t, ai.CompletedAt())
}

func TestNewActionItemValidation(t *testing.T) {
	pid, _ := domain.NewPostmortemID("pm-1")
	_, err := domain.NewActionItem(domain.NewActionItemInput{ID: "", PostmortemID: pid, Title: "x", Now: time.Now()})
	require.Error(t, err)
	_, err = domain.NewActionItem(domain.NewActionItemInput{ID: "ai", PostmortemID: pid, Title: "  ", Now: time.Now()})
	require.Error(t, err)
}

func TestActionItemUpdateStatus(t *testing.T) {
	pid, _ := domain.NewPostmortemID("pm-1")
	ai, err := domain.NewActionItem(domain.NewActionItemInput{
		ID: "ai-1", PostmortemID: pid, Title: "fix", Now: time.Date(2026, 6, 27, 10, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	done := ai.UpdateStatus(domain.ActionDone, time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC))
	require.Equal(t, domain.ActionDone, done.Status())
	require.NotNil(t, done.CompletedAt())

	reopened := done.UpdateStatus(domain.ActionInProgress, time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC))
	require.Equal(t, domain.ActionInProgress, reopened.Status())
	require.Nil(t, reopened.CompletedAt(), "rời done → xoá completedAt")
}

func TestActionItemDueDateCopy(t *testing.T) {
	pid, _ := domain.NewPostmortemID("pm-1")
	due := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	ai, err := domain.NewActionItem(domain.NewActionItemInput{
		ID: "ai-1", PostmortemID: pid, Title: "fix", DueDate: &due, Now: time.Now(),
	})
	require.NoError(t, err)
	got := ai.DueDate()
	*got = got.Add(24 * time.Hour)
	require.Equal(t, due, *ai.DueDate(), "DueDate() trả copy, không lộ con trỏ nội bộ")
}
