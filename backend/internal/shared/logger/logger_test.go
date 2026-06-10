package logger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/shared/logger"
)

func TestTraceIDRoundTrip(t *testing.T) {
	ctx := logger.ContextWithTraceID(context.Background(), "trace-123")
	require.Equal(t, "trace-123", logger.TraceIDFromContext(ctx))
	require.Empty(t, logger.TraceIDFromContext(context.Background()))
}

func TestInfoIncludesTraceID(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(&buf, "info")
	ctx := logger.ContextWithTraceID(context.Background(), "trace-abc")

	log.Info(ctx, "hello")

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	require.Equal(t, "hello", entry["message"])
	require.Equal(t, "trace-abc", entry["trace_id"])
}

func TestErrorLogsErrField(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(&buf, "error")

	log.Error(context.Background(), errors.New("boom"), "failed op")

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	require.Equal(t, "boom", entry["error"])
	require.Equal(t, "error", entry["level"])
}
