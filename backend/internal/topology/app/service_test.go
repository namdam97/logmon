package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/topology/app"
	"github.com/namdam97/logmon/backend/internal/topology/domain"
)

type fakeReader struct {
	edges []domain.Edge
	err   error
	calls int
}

func (f *fakeReader) Dependencies(context.Context, string, time.Time) ([]domain.Edge, error) {
	f.calls++
	return f.edges, f.err
}

type fakeCache struct {
	store map[string]domain.Graph
	gets  int
	sets  int
}

func newFakeCache() *fakeCache { return &fakeCache{store: map[string]domain.Graph{}} }

func (f *fakeCache) Get(_ context.Context, ws string) (domain.Graph, bool, error) {
	f.gets++
	g, ok := f.store[ws]
	return g, ok, nil
}
func (f *fakeCache) Set(_ context.Context, ws string, g domain.Graph, _ time.Duration) error {
	f.sets++
	f.store[ws] = g
	return nil
}

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func TestGetTopologyCacheMissBuildsAndCaches(t *testing.T) {
	reader := &fakeReader{edges: []domain.Edge{{Source: "a", Target: "b", CallCount: 10}}}
	cache := newFakeCache()
	svc := app.NewService(reader, cache, fixedClock{time.Unix(100, 0).UTC()})

	g, err := svc.GetTopology(context.Background(), "ws-1")
	require.NoError(t, err)
	require.Len(t, g.Nodes, 2)
	require.Equal(t, 1, reader.calls)
	require.Equal(t, 1, cache.sets)
}

func TestGetTopologyCacheHitSkipsReader(t *testing.T) {
	reader := &fakeReader{edges: []domain.Edge{{Source: "a", Target: "b", CallCount: 10}}}
	cache := newFakeCache()
	cache.store["ws-1"] = domain.BuildGraph([]domain.Edge{{Source: "x", Target: "y", CallCount: 1}}, time.Unix(1, 0).UTC())
	svc := app.NewService(reader, cache, fixedClock{time.Unix(100, 0).UTC()})

	g, err := svc.GetTopology(context.Background(), "ws-1")
	require.NoError(t, err)
	require.Equal(t, "x", g.Nodes[0].Service)
	require.Equal(t, 0, reader.calls) // cache hit → không gọi reader
}

func TestGetTopologyReaderErrorPropagates(t *testing.T) {
	reader := &fakeReader{err: errors.New("es down")}
	svc := app.NewService(reader, newFakeCache(), fixedClock{time.Unix(100, 0).UTC()})

	_, err := svc.GetTopology(context.Background(), "ws-1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "read dependencies")
}

func TestGetTopologyNilReaderEmptyGraph(t *testing.T) {
	svc := app.NewService(nil, nil, fixedClock{time.Unix(100, 0).UTC()})
	g, err := svc.GetTopology(context.Background(), "ws-1")
	require.NoError(t, err)
	require.Empty(t, g.Nodes)
	require.Equal(t, time.Unix(100, 0).UTC(), g.GeneratedAt)
}
