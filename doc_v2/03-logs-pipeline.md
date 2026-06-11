# 03 — Log Pipeline

> Thay đổi lớn nhất so với v1: **OTel Collector thay thế Filebeat + Logstash** (ADR-018), **Kafka 4.3 KRaft-only** (ADR-027), **ES data streams + ILM rollover theo size thay daily indices** (ADR-019).

---

## 1. Tại Sao Bỏ Filebeat + Logstash

Bằng chứng (benchmark VictoriaMetrics 03/2026, blog chính thức Elastic — nguồn đầy đủ trong ADR-018):

1. **Filebeat bắt đầu mất log trước ngưỡng 10K logs/s** — không đạt yêu cầu Mode B (>10K logs/s).
2. **Logstash JVM chiếm 1-4 GB RAM** chỉ để parse JSON — việc mà collector nhẹ làm được; Elastic chính thức hướng dẫn convert Logstash pipelines sang OTel Collector.
3. Elastic đã xoay trục toàn bộ ingestion sang OpenTelemetry (Elastic Agent ≥9.2 chạy EDOT Collector bên trong).
4. **Một collector cho cả 3 tín hiệu**: LogMon vốn đã cần OTel Collector cho traces — dùng luôn cho logs giảm 2 component (Filebeat, Logstash), giảm ~1.5-3.5 GB RAM, một config ngữ nghĩa thống nhất.

Lựa chọn thay thế đã cân nhắc: Fluent Bit (nhẹ nhất, nhưng thêm 1 công nghệ riêng cho logs); Vector (hiệu năng cao nhưng từng có silent log loss + FD leak trong benchmark 2026). **Quyết định: OTel Collector contrib** — vendor-neutral, một binary cho mọi tín hiệu.

---

## 2. Kiến Trúc Pipeline

```
┌─ mỗi host ─────────────────────────────────────────┐
│ Go services ──JSON──▶ stdout ──▶ Docker json-file   │
│                                       │             │
│             OTel Collector AGENT ◀────┘             │
│             • filelog receiver (tail container logs)│
│             • operators: parse JSON, severity,      │
│               timestamp, resource (service, host)   │
│             • OTLP receiver (traces từ SDK)         │
│             • processors: memory_limiter, batch     │
└──────────────────┬──────────────────────────────────┘
                   │
     Mode A: OTLP gRPC ────────────────┐
     Mode B: kafka exporter            │
              └─▶ Kafka topic otlp_logs ──▶ kafka receiver
                                       ▼
┌─ trung tâm ────────────────────────────────────────┐
│ OTel Collector GATEWAY                              │
│ • tail_sampling (traces) + spanmetrics connector    │
│ • transform/filter processors (enrich, drop noise)  │
│ • elasticsearchexporter ──▶ ES data streams         │
│ • otlp exporter ──▶ Jaeger v2                       │
│ • prometheus exporter ──▶ span metrics              │
└─────────────────────────────────────────────────────┘
```

### 2.1 Agent config (rút gọn, `infra/otel/agent.yaml`)

```yaml
receivers:
  filelog:
    include: [/var/lib/docker/containers/*/*-json.log]
    operators:
      - type: json_parser           # Docker wrapper {log, stream, time}
        parse_from: body
      - type: json_parser           # zerolog JSON bên trong
        parse_from: attributes.log
      - type: severity_parser
        parse_from: attributes.level
      - type: time_parser
        parse_from: attributes.timestamp
        layout: '%Y-%m-%dT%H:%M:%SZ'
      - type: move                  # service name → resource attr
        from: attributes.service
        to: resource["service.name"]
  otlp:
    protocols:
      grpc: { endpoint: 0.0.0.0:4317 }

processors:
  memory_limiter: { check_interval: 1s, limit_mib: 256 }
  batch: { timeout: 5s, send_batch_size: 512 }
  resourcedetection: { detectors: [system, docker] }

exporters:
  otlp/gateway:                     # Mode A
    endpoint: otel-gateway:4317
    tls: { insecure: true }        # nội bộ network; bật mTLS khi multi-host
  # kafka:                          # Mode B (profile scale)
  #   brokers: [kafka-1:9092, kafka-2:9092, kafka-3:9092]
  #   topic: otlp_logs
  #   encoding: otlp_proto

service:
  pipelines:
    logs:   { receivers: [filelog], processors: [memory_limiter, resourcedetection, batch], exporters: [otlp/gateway] }
    traces: { receivers: [otlp],    processors: [memory_limiter, batch],                    exporters: [otlp/gateway] }
```

### 2.2 Gateway — phần logs (`infra/otel/gateway.yaml`)

```yaml
exporters:
  elasticsearch:
    endpoints: ["https://elasticsearch:9200"]
    auth: { authenticator: basicauth }
    logs_dynamic_index:
      enabled: true                 # route theo data_stream.* attributes
    mapping: { mode: ecs }          # map sang ECS fields
    retry: { enabled: true, max_requests: 5 }
    sending_queue: { enabled: true, storage: file_storage }  # persistent queue chống mất log khi ES down ngắn
```

Failure handling tại gateway: `sending_queue` (file-backed) giữ logs khi ES down ngắn hạn (Mode A); Mode B thì Kafka đã là buffer chính (collector chỉ cần queue nhỏ).

---

## 3. Structured Log Format (giữ từ v1, bổ sung ECS mapping)

zerolog JSON stdout — fields bắt buộc:

```json
{
  "timestamp": "2026-06-11T10:00:00Z",
  "level": "info",
  "service": "logmon-api",
  "workspace": "backend-team",
  "trace_id": "abc-123-def-456",
  "span_id": "span-001",
  "method": "POST",
  "path": "/api/v1/alerts/rules",
  "status": 201,
  "duration_ms": 45,
  "message": "request completed",
  "caller": "adapters/http/handler.go:42"
}
```

| Quy tắc | |
|---------|---|
| Bắt buộc | `timestamp` (ISO8601 UTC), `level`, `service`, `message`; HTTP logs thêm `method/path/status/duration_ms/caller`; có trace context thì thêm `trace_id/span_id` |
| CẤM log | password, token, JWT, connection string, encryption keys, PII, request/response body, full stack trace ở production (chỉ DEBUG) |
| BẮT BUỘC log (security events) | auth attempts (success+failure), authorization failures, validation failures, admin actions, TLS failures |
| Level theo môi trường | Dev: DEBUG+ · Staging: INFO+ · Production: INFO+ (WARN+ cho service ồn, configurable) |

---

## 4. Elasticsearch: Data Streams + ILM (ADR-019)

### 4.1 Naming scheme

Theo chuẩn ES `{type}-{dataset}-{namespace}`:

```
logs-{service}-{workspace}     ← ví dụ: logs-logmon.api-default, logs-demo.order-backend.team
traces-jaeger-*                ← Jaeger v2 tự quản
```

- **KHÔNG còn date trong tên index.** Data stream tự quản backing indices (`.ds-logs-...-000001`).
- Multi-tenant (GĐ3): **data-stream-per-workspace** qua namespace — đúng khuyến nghị cho 5-50 tenants; cho phép ILM/retention riêng từng workspace. Shared index + Document-Level Security chỉ cần khi hàng trăm tenants (và DLS yêu cầu license Platinum — nếu rẽ hướng đó, cân nhắc OpenSearch nơi DLS miễn phí).

### 4.2 ILM Policy

```json
{
  "policy": {
    "phases": {
      "hot":    { "actions": { "rollover": { "max_primary_shard_size": "50gb", "max_age": "7d" },
                               "set_priority": { "priority": 100 } } },
      "warm":   { "min_age": "7d",
                  "actions": { "shrink": { "number_of_shards": 1 },
                               "forcemerge": { "max_num_segments": 1 },
                               "set_priority": { "priority": 50 } } },
      "cold":   { "min_age": "30d",
                  "actions": { "searchable_snapshot": { "snapshot_repository": "s3_repo" } } },
      "delete": { "min_age": "90d", "actions": { "delete": {} } }
    }
  }
}
```

- **Rollover theo `max_primary_shard_size: 50gb`** (khuyến nghị chính thức Elastic) — `max_age: 7d` chỉ là chặn trên, tránh index "sống mãi" khi volume thấp.
- Shard sizing: 10-50 GB/shard, < 200M docs/shard, ≤ 1000 non-frozen shards/node.
- Replicas: 0 (dev) / 1 (production 3 nodes).
- Cold phase dùng searchable snapshot lên S3 (Mode B; Mode A bỏ cold, delete sau 30d).
- Retention mặc định: hot 7d → warm 30d → cold 90d → delete. Per-workspace override qua LogPipeline BC (PUT /pipeline/ilm) — backend gọi ES ILM API.

### 4.3 Index template (rút gọn)

```json
{
  "index_patterns": ["logs-*"],
  "data_stream": {},
  "template": {
    "settings": { "number_of_shards": 1, "number_of_replicas": 1, "index.lifecycle.name": "logmon-logs" },
    "mappings": {
      "properties": {
        "@timestamp":   { "type": "date" },
        "log.level":    { "type": "keyword" },
        "service.name": { "type": "keyword" },
        "trace_id":     { "type": "keyword" },
        "span_id":      { "type": "keyword" },
        "http":         { "properties": { "method": {"type":"keyword"}, "status": {"type":"short"} } },
        "message":      { "type": "match_only_text" }
      }
    }
  }
}
```

`match_only_text` cho message — tiết kiệm ~10-20% disk so với `text` đầy đủ, vẫn full-text search được.

### 4.4 Enrichment

Transform đơn giản (rename, thêm field, drop noise) làm **tại OTel Collector** (transform processor). Enrichment cần lookup (GeoIP, mapping tĩnh) → **ES ingest pipeline**. KHÔNG dùng Logstash.

---

## 5. Kafka Buffer — Mode B (ADR-002 cập nhật + ADR-027)

| Cấu hình | Giá trị |
|----------|---------|
| Version | Kafka 4.3, **KRaft-only** (ZooKeeper đã bị xóa từ 4.0) |
| Topology | Production: **3 nodes** (controller quorum 3, combined mode không được Confluent hỗ trợ production); Staging: 1 node combined (chấp nhận rủi ro) |
| Topics | `otlp_logs` (input, 6 partitions, retention 24h), `logs-dlq` (retention 7-14 ngày) |
| Replication | RF=3, `min.insync.replicas=2` (production) |
| Consumer group | `otel-gateway` |
| Alternative | Redpanda (single binary, Kafka API drop-in) nếu muốn giảm ops — ghi nhận, không phải mặc định |

Khi nào cần Kafka: log volume > 5-10K msg/s duy trì, cần replay sau sự cố gateway/ES, hoặc burst lớn (deploy storm). Dưới ngưỡng đó Mode A + persistent queue của collector là đủ.

---

## 6. Dead Letter Queue

| Khía cạnh | Quyết định |
|-----------|-----------|
| Nguồn DLQ | (1) Log parse thất bại tại collector; (2) ES bulk reject (mapping conflict, 400) |
| Mode B | Route vào Kafka topic `logs-dlq`, kèm headers metadata: source topic/partition/offset, timestamp, error reason |
| Mode A | Ghi file local qua file exporter + expose count metric |
| Tracking | LogPipeline BC đọc DLQ → bảng `dlq_entries` (samples + count) → UI hiển thị + retry |
| **Alert theo RATE, không theo size** | `rate(logmon_dlq_messages_total[5m]) > threshold` — best practice 2026 |
| Retry | Thủ công qua API `POST /pipeline/dlq/retry` sau khi người vận hành review — **không auto-replay** |
| Retention | 7-14 ngày |

---

## 7. Quota & Bảo Vệ Pipeline (multi-layer)

```
Layer 1  OTel Agent      memory_limiter + filelog rate (drop + counter khi quá tải)
Layer 2  Kafka           producer quota 50 MB/s; consumer quota 100 MB/s (Mode B)
Layer 3  Gateway         batch + persistent queue + retry có backoff
Layer 4  Backend API     redis_rate GCRA per-workspace (search 100 req/min, tail 5 SSE concurrent)
Layer 5  Elasticsearch   ILM rollover 50gb; disk watermark mặc định; alert ES disk > 80%
```

Per-workspace ingestion quota (GĐ 3-4): đo bằng metric từ collector (`otelcol_exporter_sent_log_records` theo workspace attr) → LogPipeline BC so với quota plan → cảnh báo / drop có chủ đích (filter processor reload).

---

## 8. Log Search API (ADR-013 — giữ nguyên, cập nhật backend)

| Endpoint | Mô tả |
|----------|-------|
| `POST /api/v1/logs/search` | Full-text + filters (service, level, time range, trace_id); backend proxy ES query, **bắt buộc inject filter workspace** |
| `GET /api/v1/logs/tail` | SSE stream realtime (ES search_after polling 2s hoặc point-in-time) |
| `GET /api/v1/logs/trace/:trace_id` | Logs theo trace — query `trace_id` keyword field |
| `GET /api/v1/logs/stats` | Volume per service/level (ES aggregations) |

Bảo vệ: timeout ES 10s, max size 1000, rate limit, cache kết quả stats 30s (Redis).
