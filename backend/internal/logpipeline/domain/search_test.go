package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/logpipeline/domain"
	apperrors "github.com/namdam97/logmon/backend/internal/shared/errors"
)

func TestNewSearchCriteriaDefaults(t *testing.T) {
	// Arrange + Act: input rỗng → áp default limit, offset 0, không lỗi.
	c, err := domain.NewSearchCriteria(domain.SearchInput{})

	// Assert
	require.NoError(t, err)
	require.Equal(t, domain.DefaultLimit, c.Limit())
	require.Equal(t, 0, c.Offset())
	require.False(t, c.HasFrom())
	require.False(t, c.HasTo())
}

func TestNewSearchCriteriaNormalizesSeverity(t *testing.T) {
	c, err := domain.NewSearchCriteria(domain.SearchInput{Severity: "ERROR"})

	require.NoError(t, err)
	require.Equal(t, "error", c.Severity())
}

func TestNewSearchCriteriaTrimsAndKeepsFilters(t *testing.T) {
	from := time.Date(2026, 6, 27, 10, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)

	c, err := domain.NewSearchCriteria(domain.SearchInput{
		Service: "  userservice ",
		Query:   " timeout ",
		TraceID: "0af7651916cd43dd8448eb211c80319c",
		From:    from,
		To:      to,
		Limit:   50,
		Offset:  10,
	})

	require.NoError(t, err)
	require.Equal(t, "userservice", c.Service())
	require.Equal(t, "timeout", c.Query())
	require.Equal(t, "0af7651916cd43dd8448eb211c80319c", c.TraceID())
	require.True(t, c.HasFrom())
	require.True(t, c.HasTo())
	require.Equal(t, from, c.From())
	require.Equal(t, to, c.To())
	require.Equal(t, 50, c.Limit())
	require.Equal(t, 10, c.Offset())
}

func TestNewSearchCriteriaValidationErrors(t *testing.T) {
	from := time.Date(2026, 6, 27, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		give  domain.SearchInput
		field string
	}{
		{
			name:  "limit over max",
			give:  domain.SearchInput{Limit: domain.MaxLimit + 1},
			field: "limit",
		},
		{
			name:  "negative limit",
			give:  domain.SearchInput{Limit: -1},
			field: "limit",
		},
		{
			name:  "negative offset",
			give:  domain.SearchInput{Offset: -1},
			field: "offset",
		},
		{
			name:  "from after to",
			give:  domain.SearchInput{From: from.Add(time.Hour), To: from},
			field: "from",
		},
		{
			name:  "bad trace id",
			give:  domain.SearchInput{TraceID: "not-hex"},
			field: "trace_id",
		},
		{
			name:  "unknown severity",
			give:  domain.SearchInput{Severity: "bogus"},
			field: "severity",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewSearchCriteria(tt.give)

			require.Error(t, err)
			ve, ok := apperrors.AsValidationError(err)
			require.True(t, ok)
			require.Equal(t, tt.field, ve.Field)
		})
	}
}
