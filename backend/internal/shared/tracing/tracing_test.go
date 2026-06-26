package tracing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
)

func TestNew_DisabledWhenEndpointEmpty(t *testing.T) {
	p, err := New(context.Background(), Config{ServiceName: "test"})
	require.NoError(t, err)
	require.False(t, p.Enabled())
	// Shutdown khi tắt là no-op, không lỗi.
	require.NoError(t, p.Shutdown(context.Background()))
}

func TestNew_AlwaysSetsW3CPropagator(t *testing.T) {
	_, err := New(context.Background(), Config{ServiceName: "test"})
	require.NoError(t, err)

	prop := otel.GetTextMapPropagator()
	fields := prop.Fields()
	require.Contains(t, fields, "traceparent") // W3C TraceContext
	require.Contains(t, fields, "baggage")
}

func TestNew_EnabledWithEndpoint(t *testing.T) {
	// otlptracegrpc.New không dial ngay (lazy) → không cần collector thật.
	p, err := New(context.Background(), Config{
		ServiceName: "test",
		Endpoint:    "localhost:4317",
		Insecure:    true,
	})
	require.NoError(t, err)
	require.True(t, p.Enabled())
	t.Cleanup(func() { _ = p.Shutdown(context.Background()) })
}

func TestNew_StripsSchemeFromEndpoint(t *testing.T) {
	p, err := New(context.Background(), Config{
		ServiceName: "test",
		Endpoint:    "http://otel-agent:4317/",
		Insecure:    true,
	})
	require.NoError(t, err)
	require.True(t, p.Enabled())
	t.Cleanup(func() { _ = p.Shutdown(context.Background()) })
}

func TestStripScheme(t *testing.T) {
	tests := []struct {
		name string
		give string
		want string
	}{
		{name: "no scheme", give: "otel:4317", want: "otel:4317"},
		{name: "http", give: "http://otel:4317", want: "otel:4317"},
		{name: "https", give: "https://otel:4317", want: "otel:4317"},
		{name: "trailing slash", give: "http://otel:4317/", want: "otel:4317"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, stripScheme(tt.give))
		})
	}
}

func TestSampler(t *testing.T) {
	tests := []struct {
		name        string
		give        float64
		wantContain string
	}{
		{name: "zero ratio uses always-on root", give: 0, wantContain: "AlwaysOn"},
		{name: "negative ratio uses always-on root", give: -1, wantContain: "AlwaysOn"},
		{name: "fractional ratio uses ratio-based root", give: 0.1, wantContain: "TraceIDRatioBased"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desc := sampler(tt.give).Description()
			require.Contains(t, desc, "ParentBased")
			require.Contains(t, desc, tt.wantContain)
		})
	}
}
