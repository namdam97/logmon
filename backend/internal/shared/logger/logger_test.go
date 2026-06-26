package logger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"github.com/namdam97/logmon/backend/internal/shared/logger"
)

// ctxWithSpan trả về context mang một SpanContext W3C hợp lệ để test correlation.
func ctxWithSpan(traceHex, spanHex string) context.Context {
	tid, _ := trace.TraceIDFromHex(traceHex)
	sid, _ := trace.SpanIDFromHex(spanHex)
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    tid,
		SpanID:     sid,
		TraceFlags: trace.FlagsSampled,
	})
	return trace.ContextWithSpanContext(context.Background(), sc)
}

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

func TestSpanContextDerivesTraceAndSpanID(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(&buf, "info")
	const (
		traceHex = "0123456789abcdef0123456789abcdef"
		spanHex  = "0123456789abcdef"
	)
	ctx := ctxWithSpan(traceHex, spanHex)

	log.Info(ctx, "hello")

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	require.Equal(t, traceHex, entry["trace_id"])
	require.Equal(t, spanHex, entry["span_id"])
}

func TestSpanContextOverridesManualTraceID(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(&buf, "info")
	const traceHex = "abcdefabcdefabcdefabcdefabcdefab"
	ctx := ctxWithSpan(traceHex, "1111111111111111")
	ctx = logger.ContextWithTraceID(ctx, "manual-should-lose")

	log.Info(ctx, "hello")

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	require.Equal(t, traceHex, entry["trace_id"])
}

func TestManualTraceIDUsedWhenNoSpan(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(&buf, "info")
	ctx := logger.ContextWithTraceID(context.Background(), "trace-fallback")

	log.Info(ctx, "hello")

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	require.Equal(t, "trace-fallback", entry["trace_id"])
	require.Nil(t, entry["span_id"])
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
