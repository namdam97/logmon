package errors_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	apperrors "github.com/namdam97/logmon/backend/internal/shared/errors"
)

func TestValidationError(t *testing.T) {
	err := apperrors.NewValidationError("email", "invalid format")
	require.Equal(t, "validation: email: invalid format", err.Error())
}

func TestAsValidationError(t *testing.T) {
	tests := []struct {
		name string
		give error
		want bool
	}{
		{name: "direct validation error", give: apperrors.NewValidationError("f", "m"), want: true},
		{name: "wrapped validation error", give: fmt.Errorf("ctx: %w", apperrors.NewValidationError("f", "m")), want: true},
		{name: "sentinel is not validation", give: apperrors.ErrNotFound, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ve, ok := apperrors.AsValidationError(tt.give)
			require.Equal(t, tt.want, ok)
			if tt.want {
				require.NotNil(t, ve)
			}
		})
	}
}
