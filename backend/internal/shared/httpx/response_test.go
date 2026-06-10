package httpx_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	apperrors "github.com/namdam97/logmon/backend/internal/shared/errors"
	"github.com/namdam97/logmon/backend/internal/shared/httpx"
)

func init() { gin.SetMode(gin.TestMode) }

func TestOK(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	httpx.OK(c, http.StatusCreated, map[string]string{"id": "1"})

	require.Equal(t, http.StatusCreated, w.Code)
	var env httpx.Envelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.True(t, env.Success)
	require.Empty(t, env.Error)
}

func TestFailFromError(t *testing.T) {
	tests := []struct {
		name       string
		give       error
		wantStatus int
	}{
		{name: "not found", give: apperrors.ErrNotFound, wantStatus: http.StatusNotFound},
		{name: "conflict", give: apperrors.ErrConflict, wantStatus: http.StatusConflict},
		{name: "unauthorized", give: apperrors.ErrUnauthorized, wantStatus: http.StatusUnauthorized},
		{name: "validation", give: apperrors.NewValidationError("f", "m"), wantStatus: http.StatusBadRequest},
		{name: "unknown is 500", give: errStub("boom"), wantStatus: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			httpx.FailFromError(c, tt.give)

			require.Equal(t, tt.wantStatus, w.Code)
			var env httpx.Envelope
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
			require.False(t, env.Success)
			require.NotEmpty(t, env.Error)
		})
	}
}

type errStub string

func (e errStub) Error() string { return string(e) }
