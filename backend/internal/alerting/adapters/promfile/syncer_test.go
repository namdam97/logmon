package promfile_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/alerting/adapters/promfile"
	"github.com/namdam97/logmon/backend/internal/alerting/domain"
)

const genFile = "logmon-generated.yml"

type fakeReader struct{ rules []domain.AlertRule }

func (r *fakeReader) ByID(context.Context, domain.RuleID) (domain.AlertRule, error) {
	return domain.AlertRule{}, domain.ErrRuleNotFound
}
func (r *fakeReader) List(context.Context, string) ([]domain.AlertRule, error) { return r.rules, nil }
func (r *fakeReader) ListAll(context.Context) ([]domain.AlertRule, error)      { return r.rules, nil }

func ruleWith(t *testing.T, name, expr string) domain.AlertRule {
	t.Helper()
	id, err := domain.NewRuleID("11111111-1111-1111-1111-111111111111")
	require.NoError(t, err)
	rule, err := domain.NewAlertRule(domain.NewAlertRuleInput{
		ID:          id,
		WorkspaceID: "ws",
		Name:        name,
		Expression:  expr,
		Service:     "logmon-api",
		ForDuration: 2 * time.Minute,
		Severity:    domain.SeverityCritical,
		Annotations: map[string]string{domain.AnnotationSummary: "s", domain.AnnotationRunbookURL: "u"},
		CreatedAt:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	return rule
}

// reloadServer trả về server + con trỏ đếm số lần /-/reload được gọi.
func reloadServer(t *testing.T, status int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/-/reload" {
			calls.Add(1)
			w.WriteHeader(status)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestSyncerSuccess(t *testing.T) {
	dir := t.TempDir()
	srv, calls := reloadServer(t, http.StatusOK)
	reader := &fakeReader{rules: []domain.AlertRule{ruleWith(t, "HighErrorRate", "up == 0")}}
	s := promfile.NewSyncer(reader, dir, srv.URL)

	require.NoError(t, s.Sync(context.Background()))
	require.Equal(t, int32(1), calls.Load(), "đã gọi reload")

	content, err := os.ReadFile(filepath.Join(dir, genFile))
	require.NoError(t, err)
	require.Contains(t, string(content), "HighErrorRate")
	require.Contains(t, string(content), "severity: critical")
}

func TestSyncerInvalidPromQLNotWritten(t *testing.T) {
	dir := t.TempDir()
	srv, calls := reloadServer(t, http.StatusOK)
	// Expr non-empty (qua domain) nhưng sai cú pháp → rulefmt validate chặn.
	reader := &fakeReader{rules: []domain.AlertRule{ruleWith(t, "Bad", "(((")}}
	s := promfile.NewSyncer(reader, dir, srv.URL)

	err := s.Sync(context.Background())
	require.Error(t, err)
	require.Equal(t, int32(0), calls.Load(), "không reload khi validate fail")
	_, statErr := os.Stat(filepath.Join(dir, genFile))
	require.True(t, os.IsNotExist(statErr), "không ghi file khi validate fail")
}

func TestSyncerReloadFailureRollsBack(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, genFile)
	require.NoError(t, os.WriteFile(target, []byte("groups: []\n"), 0o644))

	srv, _ := reloadServer(t, http.StatusInternalServerError)
	reader := &fakeReader{rules: []domain.AlertRule{ruleWith(t, "HighErrorRate", "up == 0")}}
	s := promfile.NewSyncer(reader, dir, srv.URL)

	err := s.Sync(context.Background())
	require.Error(t, err)

	// Reload fail → file rollback về nội dung cũ.
	content, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "groups: []\n", string(content))
}
