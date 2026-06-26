// Package tracing dựng OpenTelemetry TracerProvider xuất span qua OTLP gRPC
// (doc_v2/04 §2, ADR-020). Endpoint rỗng → tracing tắt (no-op) để dev stack nhẹ
// và test chạy không cần collector. Tail sampling thực hiện ở gateway nên SDK
// mặc định AlwaysSample (xuất hết span, gateway quyết định giữ/bỏ).
package tracing

import (
	"context"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
)

// Config cấu hình tracing.
type Config struct {
	// ServiceName điền vào resource attribute service.name (bắt buộc khi bật).
	ServiceName string
	// ServiceVersion điền service.version nếu khác rỗng.
	ServiceVersion string
	// Endpoint là OTLP gRPC endpoint dạng host:port (scheme http/https được bỏ).
	// Rỗng → tracing tắt.
	Endpoint string
	// Insecure=true dùng gRPC plaintext (dev nội bộ); false bật TLS.
	Insecure bool
	// SampleRatio là tỷ lệ head-sampling [0,1]. <=0 → AlwaysSample (khuyến nghị —
	// tail sampling ở gateway). Dùng ParentBased để tôn trọng quyết định upstream.
	SampleRatio float64
}

// Provider giữ TracerProvider đã đăng ký global để shutdown khi service dừng.
// tp == nil nghĩa là tracing tắt.
type Provider struct {
	tp *sdktrace.TracerProvider
}

// New dựng TracerProvider theo cfg và đăng ký global (tracer + W3C propagator).
// Luôn set propagator để service đọc được traceparent của request đến, kể cả khi
// tracing tắt. Endpoint rỗng → trả Provider no-op (Enabled() == false).
func New(ctx context.Context, cfg Config) (*Provider, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	if cfg.Endpoint == "" {
		return &Provider{}, nil
	}

	opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(stripScheme(cfg.Endpoint))}
	if cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}
	exporter, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("otlp exporter: %w", err)
	}

	res, err := buildResource(cfg)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		// Batch mặc định: timeout 5s, max batch 512 span (doc_v2/04 §2.1) —
		// non-blocking, drop khi quá tải thay vì chặn request path.
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler(cfg.SampleRatio)),
	)
	otel.SetTracerProvider(tp)
	return &Provider{tp: tp}, nil
}

// Enabled báo tracing có đang bật (đã cấu hình exporter) hay không.
func (p *Provider) Enabled() bool {
	return p != nil && p.tp != nil
}

// Shutdown flush span còn lại và đóng exporter. An toàn khi tracing tắt.
func (p *Provider) Shutdown(ctx context.Context) error {
	if !p.Enabled() {
		return nil
	}
	if err := p.tp.Shutdown(ctx); err != nil {
		return fmt.Errorf("tracer shutdown: %w", err)
	}
	return nil
}

// sampler trả ParentBased(AlwaysSample) khi ratio<=0, ngược lại ParentBased theo
// tỷ lệ. ParentBased: nếu request đến đã có quyết định sampling thì tôn trọng.
func sampler(ratio float64) sdktrace.Sampler {
	if ratio <= 0 {
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	}
	return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))
}

// buildResource gắn service.name (+ version nếu có) vào resource, merge với
// resource mặc định (sdk name/version, host...).
func buildResource(cfg Config) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{semconv.ServiceName(cfg.ServiceName)}
	if cfg.ServiceVersion != "" {
		attrs = append(attrs, semconv.ServiceVersion(cfg.ServiceVersion))
	}
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL, attrs...),
	)
	if err != nil {
		return nil, fmt.Errorf("build resource: %w", err)
	}
	return res, nil
}

// stripScheme bỏ tiền tố http:// hoặc https:// để còn host:port cho gRPC dial.
func stripScheme(endpoint string) string {
	endpoint = strings.TrimPrefix(endpoint, "https://")
	endpoint = strings.TrimPrefix(endpoint, "http://")
	return strings.TrimSuffix(endpoint, "/")
}
