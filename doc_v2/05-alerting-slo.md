# 05 — Alerting & SLO

> Nguyên tắc phân vai (ADR-024): **Prometheus đánh giá** rules · **Alertmanager route + dedup + silence** · **LogMon quản lý vòng đời** (CRUD, sync, track, ack, SLO engine). Không tự build evaluation engine.

---

## 1. Rule Sync Pipeline (ADR-024)

Prometheus **không có API ghi rule** (và upstream từ chối thêm). Pattern chuẩn cho platform lưu rules trong DB:

```
PostgreSQL (source of truth)
   │  rule thay đổi (event AlertRuleCreated/Updated/Deleted)
   ▼
Renderer (alerting/adapters/promfile)
   │  sinh YAML rule groups (kèm hash + version trong comment header)
   ▼
Validate: `promtool check rules <file>`        ← reject nếu PromQL/YAML sai
   ▼
Atomic write: ghi temp file → rename vào infra/prometheus/rules/generated/
   ▼
POST http://prometheus:9090/-/reload           ← cần --web.enable-lifecycle
   ▼
Verify: GET /api/v1/rules — đối chiếu hash; lỗi → rollback file trước đó + alert
```

Quy tắc:
- Rule files là **build artifact** — có version/hash, rollback được; thư mục `rules/generated/` không sửa tay (file tĩnh đặt ở `rules/static/`).
- Reload thất bại nếu **bất kỳ** rule file nào invalid → validate TRƯỚC khi ghi là bắt buộc.
- Validation lúc tạo rule trong API: parse PromQL (thư viện `prometheus/prometheus/promql/parser`) + enforce labels bắt buộc.
- Trên Kubernetes (GĐ 4): đổi adapter sang sinh `PrometheusRule` CR, prometheus-operator lo reload. Multi-tenant lớn về sau: Mimir Ruler API. Giữ interface `ports.RuleSyncer` để swap.

### Labels/annotations bắt buộc trên MỌI alert rule

| Field | Bắt buộc | Ví dụ |
|-------|----------|-------|
| `severity` | ✅ | `critical` (page) / `warning` (ticket) / `info` (ghi nhận) |
| `service` | ✅ | `logmon-api` |
| `team` | ✅ (GĐ3: map từ workspace) | `backend-team` |
| `runbook_url` (annotation) | ✅ | URL pattern cố định `https://<wiki>/runbooks/<alertname>` |
| `summary` (annotation) | ✅ | Mô tả ngắn có template `{{ $labels.service }}` |

Enforce bằng validation khi tạo/sửa rule — không có runbook_url thì không cho activate.

### Quy tắc chất lượng alert (chống alert fatigue)

1. **Symptom-based trước cause-based**: page theo điều user cảm nhận (error rate, latency, availability). Nguyên nhân (CPU cao, disk đầy) → warning/dashboard.
2. **Mọi page phải actionable** — nếu phản ứng là chạy script thì tự động hóa thay vì page.
3. `for` duration: critical ≥ 1m, warning ≥ 5m. KHÔNG alert trên raw counter — luôn `rate()`/`increase()`.
4. Severity = hành động: `critical` → đánh thức người (PagerDuty); `warning` → xử lý giờ hành chính; `info` → không notify.

---

## 2. Alertmanager 0.32

### 2.1 Cấu hình production (rút gọn)

```yaml
route:
  group_by: [alertname, service]
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 4h
  receiver: slack-default
  routes:
    - matchers: [alertname = "Watchdog"]      # deadman switch — route riêng, KHÔNG gom nhóm
      receiver: deadman
      group_wait: 0s
      group_interval: 1m
      repeat_interval: 50s
    - matchers: [severity = "critical"]
      receiver: page                            # PagerDuty + Slack critical
      group_wait: 10s
      repeat_interval: 1h
    - matchers: [severity = "warning"]
      receiver: ticket                          # Slack/Email
      repeat_interval: 12h

inhibit_rules:
  # Critical đè MỌI warning cùng service (equal theo service, không theo
  # alertname — critical ServiceDown phải nén được warning HighLatencyP95).
  - source_matchers: [severity = "critical"]
    target_matchers: [severity = "warning"]
    equal: [service]

receivers:
  - name: page
    pagerduty_configs: [{ routing_key: <secret>, severity: critical }]
    slack_configs: [{ channel: "#alerts-critical", send_resolved: true }]
  - name: ticket
    slack_configs: [{ channel: "#alerts", send_resolved: true }]
  - name: deadman
    webhook_configs: [{ url: "https://hc-ping.com/<uuid>", send_resolved: false }]
  - name: slack-default
    slack_configs: [{ channel: "#alerts" }]
  - name: logmon                                # mọi alert cũng bắn vào LogMon backend
    webhook_configs: [{ url: "http://logmon-api:8080/api/v1/alerts/webhook", max_alerts: 100 }]
```

(Receiver `logmon` được thêm vào route gốc với `continue: true` để mọi alert đều được LogMon track song song với notify.)

- **HA (Mode B)**: 2 replicas gossip (`--cluster.peer`); Prometheus cấu hình gửi tới **CẢ HAI** instance (không load-balance) — Alertmanager tự dedup qua notification log.
- Silence từ LogMon UI → gọi Alertmanager API v2 (`POST /api/v2/silences`) + lưu bản ghi ở DB để audit.

### 2.2 Webhook receiver (alerting BC)

Alertmanager webhook payload v4: `{version, groupKey, status: firing|resolved, alerts: [{status, labels, annotations, startsAt, endsAt, fingerprint, generatorURL}]}`.

Quy tắc xử lý tại `app/command/ingest_webhook.go`:
- **Idempotency key = `fingerprint` + `startsAt`** — webhook có thể gửi lặp.
- `status=firing` → upsert `alert_instances` (status firing); `status=resolved` → đóng instance + emit `AlertResolved`.
- Critical firing liên tục > 5 phút → emit event cho incident BC auto-create.
- Endpoint không auth bằng JWT user — dùng **bearer token nội bộ** riêng (secret giữa Alertmanager và LogMon), network internal.

---

## 3. Bộ Alert Nền (static rules — GĐ 1)

| Alert | Expression | For | Severity |
|-------|------------|-----|----------|
| ServiceDown | `up{job="logmon-services"} == 0` | 1m | critical |
| HighErrorRate | `rate(logmon_http_requests_total{status=~"5.."}[5m]) / rate(logmon_http_requests_total[5m]) > 0.05` | 2m | critical |
| HighLatencyP95 | `histogram_quantile(0.95, rate(logmon_http_request_duration_seconds_bucket[5m])) > 1` | 5m | warning |
| ESDiskHigh | `elasticsearch_filesystem_data_used_percent > 80` | 10m | warning |
| PGConnHigh | `pg_stat_activity_count / pg_settings_max_connections > 0.8` | 5m | warning |
| KafkaConsumerLag (Mode B) | `kafka_consumergroup_lag > 10000` | 5m | warning |
| CollectorQueueFull | `otelcol_exporter_queue_size / otelcol_exporter_queue_capacity > 0.8` | 5m | warning |
| DLQRateHigh | `rate(logmon_dlq_messages_total[5m]) > 10` | 5m | warning |
| OutboxLag | `logmon_outbox_lag_seconds > 30` | 5m | warning |
| **Watchdog** | `vector(1)` | — | none (deadman) |

---

## 4. SLO Engine (GĐ 3)

### 4.1 Mô hình

- SLO = `{service, SLI type, target, window}`. SLI types GĐ 3: **availability** (1 − error ratio) và **latency** (tỉ lệ request nhanh hơn threshold). Cả hai tính từ `logmon_http_requests_total` / duration histogram (hoặc spanmetrics).
- Window mặc định: **28d rolling** (bội số tuần — tránh dao động theo ngày trong tuần; 30d cũng hỗ trợ).
- Mode A (không Thanos): chỉ cho phép window ≤ 14d, UI cảnh báo rõ.

### 4.2 Multiwindow Multi-Burn-Rate (ADR-025 — công thức chuẩn Google SRE Workbook Ch.5)

`burn rate = tốc độ tiêu thụ error budget so với mức "vừa khít" (1x = hết budget đúng cuối window)`

| Severity | Budget tiêu thụ để trigger | Long window | Short window | Burn rate factor |
|----------|---------------------------|-------------|--------------|------------------|
| **page** (critical) | 2% | 1h | 5m | **14.4** |
| **page** (critical) | 5% | 6h | 30m | **6** |
| **ticket** (warning) | 10% | 3d | 6h | **1** |

Điều kiện alert = long window **AND** short window (short ≈ 1/12 long — alert tắt nhanh khi lỗi đã ngừng):

```promql
# SLO availability 99.9% (error budget = 0.001), service X
(
  slo:errors:ratio_rate1h{slo="X"}  > (14.4 * 0.001)
and
  slo:errors:ratio_rate5m{slo="X"}  > (14.4 * 0.001)
)
or
(
  slo:errors:ratio_rate6h{slo="X"}  > (6 * 0.001)
and
  slo:errors:ratio_rate30m{slo="X"} > (6 * 0.001)
)
# → severity: critical (page)
```

### 4.3 Rule generation

Khi SLO được tạo/sửa, SLO BC sinh (qua cùng Rule Sync Pipeline ở mục 1):

1. **Recording rules** — `slo:errors:ratio_rateXX` cho các window 5m, 30m, 1h, 6h, 3d (+ 28d cho budget tính toán). Tham khảo cách đặt window của Sloth/Pyrra (cùng pattern Workbook).
2. **Alerting rules** — 2 cặp page + 1 cặp ticket như trên.
3. **Budget snapshot job** — goroutine định kỳ (5 phút) query Prometheus/Thanos, ghi `slo_snapshots` (current SLI, budget remaining, burn rates) → read model cho UI/API, emit `BudgetExhausted` khi budget < 10%.

Naming convention recording rules: `slo:<sli>:<aggregation>_rate<window>{slo, service, workspace}`.

### 4.4 API chính (chi tiết trong 07)

`POST /slos` (define) · `GET /slos/:id/budget` (remaining, burn rates 1h/6h/24h) · `GET /slos/compliance` (tổng quan). Hỗ trợ import/export **OpenSLO v1** ở mức nice-to-have.

---

## 5. Meta-Monitoring (ADR-026 — mới trong v2)

"Ai canh người canh gác" — LogMon không được tự canh chính nó là tầng duy nhất:

| Tầng | Cơ chế |
|------|--------|
| 1. Watchdog deadman switch | Alert `Watchdog` (`vector(1)`) luôn firing → route riêng → **healthchecks.io** (period 5m, grace 10m). Ping ngừng = pipeline Prometheus→Alertmanager→internet chết → healthchecks.io tự báo qua kênh độc lập |
| 2. External uptime check | Dịch vụ ngoài (UptimeRobot/healthchecks) check HTTPS endpoints: Grafana, Prometheus, LogMon API `/health` |
| 3. Self metrics | Collector `:8888`, Prometheus self-scrape, `logmon_outbox_lag_seconds`, DLQ rate — dashboard `pipeline-health` |

Lưu ý: Grafana OnCall OSS đã archived (03/2026) — không dùng cho heartbeat; dùng healthchecks.io (free tier đủ) hoặc PagerDuty Dead Man's Snitch.
