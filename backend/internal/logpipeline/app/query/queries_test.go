package query_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/logpipeline/app/query"
	"github.com/namdam97/logmon/backend/internal/logpipeline/domain"
	apperrors "github.com/namdam97/logmon/backend/internal/shared/errors"
)

type fakeSearcher struct {
	got    domain.SearchCriteria
	result domain.SearchResult
	err    error
}

func (f *fakeSearcher) Search(_ context.Context, c domain.SearchCriteria) (domain.SearchResult, error) {
	f.got = c
	return f.result, f.err
}

func TestSearchValidatesInputBeforeCallingSearcher(t *testing.T) {
	// Arrange: input sai (limit âm) — searcher KHÔNG được gọi.
	searcher := &fakeSearcher{}
	q := query.NewLogQueries(searcher)

	// Act
	_, err := q.Search(context.Background(), domain.SearchInput{Limit: -1})

	// Assert
	require.Error(t, err)
	_, ok := apperrors.AsValidationError(err)
	require.True(t, ok)
	require.Equal(t, domain.SearchCriteria{}, searcher.got)
}

func TestSearchPassesCriteriaAndReturnsResult(t *testing.T) {
	searcher := &fakeSearcher{
		result: domain.SearchResult{
			Entries: []domain.LogEntry{{Body: "hello", Service: "userservice"}},
			Total:   1,
		},
	}
	q := query.NewLogQueries(searcher)

	res, err := q.Search(context.Background(), domain.SearchInput{Service: "userservice", Limit: 25})

	require.NoError(t, err)
	require.Equal(t, 1, res.Total)
	require.Len(t, res.Entries, 1)
	require.Equal(t, "userservice", searcher.got.Service())
	require.Equal(t, 25, searcher.got.Limit())
}

func TestSearchWrapsSearcherError(t *testing.T) {
	searcher := &fakeSearcher{err: errors.New("es down")}
	q := query.NewLogQueries(searcher)

	_, err := q.Search(context.Background(), domain.SearchInput{})

	require.Error(t, err)
	require.Contains(t, err.Error(), "search logs")
}
