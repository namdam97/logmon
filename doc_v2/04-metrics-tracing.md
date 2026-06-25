# 04 — Metrics & Distributed Tracing

---

## 1. Metrics — Prometheus 3.12

### 1.1 Cấu hình nền

| Mục | Giá trị | Ghi chú |
|-----|---------|---------|
| Scrape interval | 15s (Go services) / 60s (exporters) | |
| Retention local | 15d | Mode B: Thanos kéo dài lên 1 năm |
| Feature flags | `--enable-feature=exemplar-storage` | Exemplar → click sang trace |
| Native histograms | Dùng cho histogram mới (stable từ v3.8) | Giữ classic buckets song song trong giai đoạn chuyển tiếp |
| OTLP ingestion | `/api/v1/otlp/v1/metrics` available | Dùng khi cần đẩy metrics từ OTel SDK không qua scrape |
| Lifecycle | `--web.enable-lifecycle` | Bắt buộc — rule sync cần `POST /-/reload` (ADR-024) |

### 1.2 Exporters

| Source | Exporter | Port | Interval |
|--------|----------|------|----------|
| Go services | built-in `/metrics` (promauto) | 9090+ | 15s |
| Linux host | node_exporter | 9100 | 60s |
| PostgreSQL | postgres_exporter | 9187 | 60s |
| Redis | redis_exporter | 9121 | 60s |
| Kafka (Mode B) | kafka_exporter | 9308 | 60s |
| Elasticsearch | elasticsearch_exporter | 9114 | 60s |
| OTel Collector | tự expose `:8888/metrics` | 8888 | 60s |

### 1.3 Quy tắc naming (giữ từ v1)

- `snake_case`, prefix `logmon_`, Counter suffix `_total`.
- Labels cho phép: `method`, `path` (template, không raw), `status_code`, `service`, `workspace`.
- **CẤM high-cardinality labels**: `user_id`, `request_id`, `trace_id`, `session_id`.
- Metrics chuẩn mỗi service: `logmon_http_requests_total`, `logmon_http_request_duration_seconds` (histogram), `logmon_http_requests_in_flight` (gauge).

### 1.4 Thanos — long-term metrics (Mode B, ADR-011 — research 2026 xác nhận vẫn là best practice)

```
Grafana ─▶ Thanos Query ─┬─▶ Prometheus (qua Sidecar, 0-15d, full resolution)
                          └─▶ Thanos Store Gateway ─▶ S3/B2 (15d-1y, downsampled)
Thanos Sidecar: upload TSDB blocks mỗi 2h ─▶ S3
Thanos Compactor (single replica): dedup + downsample raw→5m→1h
```

Retention khuyến nghị: **raw 30d · 5m-downsample 180d · 1h-downsample 1 năm**.

Lý do cần: SLO window 28-30d cần dữ liệu vượt 15d local. Mode A không có Thanos → SLO chỉ chạy window ≤ 14d (chấp nhận được cho giai đoạn đầu, ghi rõ trong UI).

---

## 2. Tracing — OTel SDK + Jaeger v2

### 2.1 Instrumentation (Go services, ~30 dòng/service)

| Thành phần | Thư viện | Ghi chú |
|------------|----------|---------|
| HTTP server | `go.opentelemetry.io/contrib/.../otelgin` | Middleware #2 trong chain |
| PostgreSQL | `github.com/exaring/otelpgx` | Tracer cho pgx/v5 |
| Redis | `github.com/redis/go-redis/extra/redisotel/v9` | Chính chủ go-redis |
| HTTP client | `otelhttp` transport | Service-to-service propagation |
| Propagation | W3C Trace Context (`traceparent`) | Mặc định OTel SDK |
| Export | OTLP gRPC → Agent collector, batch 5s/512 spans, non-blocking | |
| Logs correlation | zerolog hook đọc `trace.SpanContextFromContext(ctx)` → inject `trace_id`, `span_id` | OTel Go Logs SDK còn Beta → tiếp tục zerolog (ADR-010 cập nhật) |

### 2.2 Tail Sampling (tại Gateway)

```yaml
processors:
  tail_sampling:
    decision_wait: 10s          # tăng lên 30s nếu có trace dài
    num_traces: 50000           # RAM tỷ lệ thuận — theo dõi memory collector
    policies:
      - name: errors-always
        type: status_code
        status_code: { status_codes: [ERROR] }
      - name: slow-requests
        type: latency
        latency: { threshold_ms: 1000 }
      - name: drop-health-checks
        type: string_attribute
        string_attribute:
          key: http.target
          values: ["/health", "/ready", "/metrics"]
          enabled_regex_matching: true
          invert_match: true
      - name: probabilistic-default
        type: probabilistic
        probabilistic: { sampling_percentage: 10 }
```

Ràng buộc quan trọng: **mọi span của một trace phải về cùng một gateway instance**. 1 gateway → không vấn đề. Khi scale nhiều gateway → thêm `loadbalancing` exporter (routing theo traceID) ở tầng trước.

### 2.3 Span Metrics (RED tự động)

Dùng **spanmetrics CONNECTOR** (processor cũ đã bị gỡ — v1 dùng processor là lỗi thời):

```yaml
connectors:
  spanmetrics:
    histogram:
      exponential: { max_size: 160 }    # exponential histogram → Prometheus native histogram
    exemplars: { enabled: true }         # exemplar trỏ về trace_id
    dimensions:
      - name: http.method
      - name: http.status_code

service:
  pipelines:
    traces:  { receivers: [otlp], processors: [tail_sampling, batch], exporters: [otlp/jaeger, spanmetrics] }
    metrics/spans: { receivers: [spanmetrics], exporters: [prometheus] }
```

Kết quả: RED metrics (`duration`, `calls`) per service/endpoint tự động từ traces — giảm manual instrumentation, kèm exemplars để click từ latency panel sang trace.

### 2.4 Jaeger v2 (ADR-020)

| Mục | Giá trị |
|-----|---------|
| Version | v2.19+ — **bắt buộc v2** (v1 EOL 31/12/2025) |
| Kiến trúc | Jaeger v2 = distribution của OTel Collector; nhận OTLP native |
| Storage | Elasticsearch (dùng chung cluster với logs, index prefix `jaeger-`) |
| Retention | 7 ngày (volume cao) — ILM riêng cho indices jaeger |
| UI | Jaeger UI :16686 + Grafana Jaeger datasource |
| Tương lai | ClickHouse backend (alpha từ v2.18, nén ~8.6x) — theo dõi, chưa dùng. Grafana Tempo là alternative nếu chuyển trace storage sang object storage (ADR-020) |

---

## 3. Correlation — Liên Kết 3 Trụ Cột

| Từ → Đến | Cơ chế | Cấu hình |
|----------|--------|----------|
| Metrics → Traces | **Exemplars** | Prometheus exemplar-storage + spanmetrics exemplars; Grafana panel "Query with exemplars" → nút mở trace |
| Logs → Traces | `trace_id` field | Grafana ES datasource **derived field**: regex `trace_id`, link sang Jaeger datasource |
| Traces → Logs | Jaeger/Grafana trace view | Grafana "trace to logs": query ES theo `trace_id` + time range của span |
| Metrics → Logs | Time range + service label | Grafana dashboard links cùng `service` + khoảng thời gian |

Đây là acceptance test của GĐ 2: từ một panel error-rate spike → click exemplar → trace waterfall → click span → logs của đúng request đó. Demo được flow này nghĩa là correlation hoạt động.

---

## 4. Grafana

| Mục | Quyết định |
|-----|-----------|
| Version | 13.1.x (Git Sync GA từ v13; 13.1.0 ra 23/06/2026) |
| Datasources (provisioned) | Prometheus (Mode A) / Thanos Query (Mode B), Elasticsearch, Jaeger |
| Dashboards-as-code | JSON trong `infra/grafana/dashboards/`, provisioning tự load; mọi thay đổi qua git (Grafana 13 Git Sync GA — dùng được ngay) |
| Auth | Grafana đứng sau reverse proxy; tài khoản riêng (GĐ1) — SSO/proxy auth từ LogMon là việc tương lai |

Dashboard chuẩn theo persona (giữ từ v1):

| Dashboard | Persona | Nội dung chính |
|-----------|---------|----------------|
| `service-overview` | Developer | RED per service (từ spanmetrics + http metrics), exemplar links |
| `logs-explorer` | Developer | Log search, level filter, trace_id link |
| `traces-explorer` | Developer | Latency breakdown, dependency graph |
| `infrastructure` | DevOps | node/postgres/redis/kafka/es exporters |
| `slo-dashboard` | SRE | Error budget, burn rate per SLO (GĐ 3) |
| `alerting-overview` | All | Active alerts, history, silences |
| `pipeline-health` | DevOps | Collector throughput, queue size, DLQ rate, ES indexing rate — **mới trong v2** (meta-monitoring) |
