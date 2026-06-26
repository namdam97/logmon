package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/alerting/domain"
)

func validSilenceInput() domain.NewSilenceInput {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return domain.NewSilenceInput{
		Matchers: []domain.MatcherInput{
			{Name: "alertname", Value: "HighErrorRate", IsEqual: true},
		},
		StartsAt:  start,
		EndsAt:    start.Add(2 * time.Hour),
		CreatedBy: "11111111-1111-1111-1111-111111111111",
		Comment:   "đang điều tra sự cố",
	}
}

func TestNewSilence_Valid(t *testing.T) {
	s, err := domain.NewSilence(validSilenceInput())

	require.NoError(t, err)
	require.Len(t, s.Matchers(), 1)
	require.Equal(t, "alertname", s.Matchers()[0].Name())
	require.True(t, s.Matchers()[0].IsEqual())
	require.Equal(t, "đang điều tra sự cố", s.Comment())
	require.Equal(t, "11111111-1111-1111-1111-111111111111", s.CreatedBy())
}

func TestNewSilence_MatchersReturnsCopy(t *testing.T) {
	s, err := domain.NewSilence(validSilenceInput())
	require.NoError(t, err)

	got := s.Matchers()
	got[0] = domain.NewSilenceMatcher("tampered", "x", false, false)

	require.Equal(t, "alertname", s.Matchers()[0].Name(), "mutation bản sao không ảnh hưởng silence gốc")
}

func TestNewSilence_RejectsInvalid(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		give  func(in domain.NewSilenceInput) domain.NewSilenceInput
		field string
	}{
		{
			name:  "no matchers",
			give:  func(in domain.NewSilenceInput) domain.NewSilenceInput { in.Matchers = nil; return in },
			field: "matchers",
		},
		{
			name: "matcher without name",
			give: func(in domain.NewSilenceInput) domain.NewSilenceInput {
				in.Matchers = []domain.MatcherInput{{Name: "", Value: "x", IsEqual: true}}
				return in
			},
			field: "matchers",
		},
		{
			name:  "empty createdBy",
			give:  func(in domain.NewSilenceInput) domain.NewSilenceInput { in.CreatedBy = ""; return in },
			field: "createdBy",
		},
		{
			name:  "empty comment",
			give:  func(in domain.NewSilenceInput) domain.NewSilenceInput { in.Comment = ""; return in },
			field: "comment",
		},
		{
			name: "endsAt equals startsAt",
			give: func(in domain.NewSilenceInput) domain.NewSilenceInput {
				in.EndsAt = start
				in.StartsAt = start
				return in
			},
			field: "endsAt",
		},
		{
			name: "endsAt before startsAt",
			give: func(in domain.NewSilenceInput) domain.NewSilenceInput {
				in.EndsAt = start
				in.StartsAt = start.Add(time.Hour)
				return in
			},
			field: "endsAt",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewSilence(tt.give(validSilenceInput()))

			var ve *domain.ValidationError
			require.True(t, errors.As(err, &ve), "want ValidationError, got %v", err)
			require.Equal(t, tt.field, ve.Field)
		})
	}
}
