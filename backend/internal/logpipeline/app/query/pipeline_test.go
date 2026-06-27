package query_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/logpipeline/app/query"
	"github.com/namdam97/logmon/backend/internal/logpipeline/domain"
)

type fakeCfg struct{ cfg *domain.PipelineConfig }

func (f fakeCfg) Get(_ context.Context, _ string) (domain.PipelineConfig, error) {
	if f.cfg == nil {
		return domain.PipelineConfig{}, domain.ErrPipelineConfigNotFound
	}
	return *f.cfg, nil
}
func (f fakeCfg) Upsert(context.Context, domain.PipelineConfig) error { return nil }

type fakeDLQReader struct {
	entries []domain.DLQEntry
	counts  map[string]int
}

func (f fakeDLQReader) List(context.Context, string, string, int) ([]domain.DLQEntry, error) {
	return f.entries, nil
}
func (f fakeDLQReader) CountByStatus(context.Context, string) (map[string]int, error) {
	return f.counts, nil
}

type fakeHealth struct{ s domain.HealthStatus }

func (f fakeHealth) Check(context.Context) domain.HealthStatus { return f.s }

type fakeDS struct{ stats []domain.DataStreamStat }

func (f fakeDS) Stats(context.Context, string) ([]domain.DataStreamStat, error) { return f.stats, nil }

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func TestStatusDefaultsWhenNoConfig(t *testing.T) {
	q := query.NewPipelineQueries(fakeCfg{}, fakeDLQReader{},
		fakeHealth{s: domain.HealthStatus{Elasticsearch: "up"}},
		fakeDS{stats: []domain.DataStreamStat{{Name: "logs-a"}, {Name: "logs-b"}}},
		fixedClock{t: time.Unix(1, 0).UTC()})

	view, err := q.Status(context.Background(), "ws-1", "default")
	require.NoError(t, err)
	require.Equal(t, "A", view.Mode) // mặc định Mode A
	require.Equal(t, "up", view.Health.Elasticsearch)
	require.Equal(t, 2, view.DataStreams)
}

func TestListDLQ(t *testing.T) {
	now := time.Unix(1, 0).UTC()
	q := query.NewPipelineQueries(fakeCfg{}, fakeDLQReader{
		entries: []domain.DLQEntry{domain.ReconstructDLQEntry(1, "ws-1", "r", "e", "s", 0, domain.DLQPending, now, nil)},
		counts:  map[string]int{"pending": 1},
	}, nil, nil, fixedClock{t: now})

	entries, counts, err := q.ListDLQ(context.Background(), "ws-1", "", 50)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, 1, counts["pending"])
}
