# 02 — Kiến Trúc Backend

> Modular monolith `logmon-api` chứa các Bounded Context. Hai mô hình kiến trúc tùy complexity. Layer direction strict. Cross-BC qua domain events trên transactional outbox.

---

## 1. Bounded Contexts

| BC | Pattern | Giai đoạn | Lý do pattern |
|----|---------|-----------|---------------|
| `identity/` | Clean Architecture | GĐ 1 (một phần đã có) | Auth, users, workspaces, RBAC — CRUD + policy, domain đơn giản |
| `alerting/` | Clean Arch + DDD + CQRS | GĐ 2 | Business rules thật: rule lifecycle, ack, silence, inhibition tracking |
| `logpipeline/` | Clean Arch + DDD + CQRS | GĐ 2-3 | Mode switching, DLQ retry, ILM policy management |
| `slo/` | Clean Arch + DDD + CQRS | GĐ 3 | Error budget, burn rate, rule generation |
| `incident/` | Clean Arch + DDD + CQRS | GĐ 3 | Lifecycle state machine, on-call, escalation, MTTR |
| `notification/` | Clean Architecture | GĐ 3 | Multi-channel delivery — pipeline đơn giản, không cần CQRS |
| `shared/` | Shared Kernel | GĐ 1 (đã có phần lớn) | Cross-cutting: auth, errors, logger, metrics, middleware, tracing, resilience, eventbus, httpx |

**Đã loại khỏi platform:** `order/` — chuyển thành `examples/demo-order/` (ADR-029). `user/` hiện tại trong repo **tiến hóa thành `identity/`** (thêm workspaces + RBAC ở GĐ 3, giữ nguyên cấu trúc hiện có ở GĐ 1-2).

---

## 2. Layer Direction (strict, một chiều)

```
adapters → ports ← app → domain
```

| Layer | Được import | Tuyệt đối KHÔNG |
|-------|-------------|------------------|
| `domain/` | Go stdlib ONLY | gin, pgx, redis, prometheus, BC khác |
| `app/` | `domain/`, `ports/` cùng BC; `shared/errors` | adapters, infrastructure |
| `ports/` | `domain/` cùng BC (chỉ interfaces) | implementation |
| `adapters/` | `ports/`, `domain/` cùng BC + infra libs | `app/` của BC khác |
| `cmd/` | mọi thứ (wiring) | — |

- **Không cross-BC imports** — giao tiếp qua domain events (mục 5) hoặc shared kernel.
- Enforce bằng `golangci-lint` depguard rules + code review.
- Compile-time interface check ở đầu mỗi adapter: `var _ ports.AlertRuleRepository = (*PostgresAlertRepo)(nil)`.

### Clean Architecture (identity, notification)

```
HTTP → shared/middleware chain → adapters/http/handler → app/<usecase> → domain
                                                              ↓
                                                    ports/<interface> ← adapters/postgres|redis|smtp
```

### DDD + CQRS (alerting, slo, logpipeline, incident)

```
                       adapters/http/handler
                      ┌──────────┴──────────┐
            WRITE (Command)            READ (Query)
            app/command/*              app/query/*
                 ↓                          ↓
            domain/ (Aggregate,       ports/read_model.go
            VO, Domain Events)        (denormalized views, cache Redis)
                 ↓
            ports/repository.go + ports/event_publisher.go
                 ↓
            adapters/postgres (cùng TX: data + outbox INSERT)
```

Lý do CQRS: monitoring có read:write ~100:1 — read side tối ưu riêng (cache active alerts, materialized SLO snapshots) không ảnh hưởng write side (ADR-008).

---

## 3. Middleware Chain (thứ tự bắt buộc)

| # | Middleware | Chức năng | GĐ |
|---|-----------|-----------|-----|
| 1 | `recovery` | Catch panic, log stack, HTTP 500 | 1 ✅ |
| 2 | `tracing` | OTel: extract/inject W3C traceparent, tạo request span | 2 |
| 3 | `logging` | Inject trace_id + span_id, log request/response + duration | 1 ✅ (thêm trace fields ở GĐ 2) |
| 4 | `metrics` | `logmon_http_requests_total`, duration histogram, in-flight | 1 ✅ |
| 5 | `ratelimit` | redis_rate GCRA per-workspace/per-IP | 1 ✅ (per-IP), 3 (per-workspace) |
| 6 | `auth` | Verify JWT access token | 1 ✅ |
| 7 | `csrf` | Double-submit token cho state-changing endpoints | 2 |
| 8 | `workspace` | Extract + validate workspace_id | 3 |
| 9 | `rbac` | Check role trong workspace | 3 |

(✅ = đã có trong repo hiện tại.)

---

## 4. Domain Model Chính Theo BC

### 4.1 Alerting

| DDD Concept | Implementation |
|-------------|----------------|
| Aggregate Root | `AlertRule` — lifecycle: draft → active → (synced to Prometheus) → disabled |
| Entity | `AlertInstance` — một lần firing (từ Alertmanager webhook, key = fingerprint) |
| Entity | `Silence` — time window tắt notification |
| Value Object | `Severity` (critical/warning/info), `PromQLExpr` (validate qua promtool/parser) |
| Domain Events | `AlertRuleCreated/Updated/Deleted`, `AlertFired`, `AlertResolved`, `AlertAcknowledged` |

**Ranh giới trách nhiệm (quan trọng — ADR-024):** LogMon KHÔNG đánh giá rule. Prometheus đánh giá; Alertmanager route; LogMon: (1) CRUD rules trong PostgreSQL, (2) render + sync rule files sang Prometheus, (3) nhận webhook từ Alertmanager để track instances, (4) quản lý ack/silence (silence đẩy sang Alertmanager API v2).

### 4.2 SLO

| Concept | Implementation |
|---------|----------------|
| Aggregate Root | `ServiceLevelObjective` — target, window (28d/30d), SLI type (availability/latency) |
| Value Object | `ErrorBudget`, `BurnRate` |
| Domain Events | `SLODefined`, `BudgetExhausted`, `BurnRateExceeded` |
| Hành vi chính | Generate recording + alerting rules theo multiwindow multi-burn-rate (xem 05) |

### 4.3 LogPipeline

| Concept | Implementation |
|---------|----------------|
| Aggregate Root | `Pipeline` — mode (A/B), trạng thái health |
| Value Object | `PipelineMode`, `RetentionPolicy` (hot/warm/cold/delete days) |
| Entity | `DeadLetter` — entry trong DLQ, retryable |
| Domain Events | `PipelineModeChanged`, `DLQRateExceeded` |

### 4.4 Incident

| Concept | Implementation |
|---------|----------------|
| Aggregate Root | `Incident` — state machine: open → triaged → assigned → mitigating → resolved → postmortem_pending → closed |
| Entity | `TimelineEntry`, `Postmortem` (+ action items) |
| Value Object | `IncidentSeverity` (SEV1-4), `OnCallSchedule` |
| Domain Events | `IncidentCreated/Assigned/Escalated/Resolved`, `PostmortemCompleted` |

---

## 5. Domain Events & Transactional Outbox (ADR-016)

### 5.1 Event Catalog (cross-BC)

```
alerting:  AlertFired(critical, >5m)   → incident: AutoCreateIncident
           AlertFired                  → notification: NotifyChannels
           AlertResolved               → incident: AutoResolveIncident
                                       → notification: NotifyResolution
slo:       BudgetExhausted(<10%)       → alerting: CreateCriticalAlert
                                       → incident: AutoCreateIncident(SEV2)
           BurnRateExceeded            → notification: NotifyBudgetWarning
logpipeline: PipelineModeChanged       → alerting: UpdatePipelineAlerts
             DLQRateExceeded           → alerting: CreateWarningAlert
incident:  IncidentCreated             → notification: NotifyOnCall (escalation policy)
           IncidentAssigned/Escalated  → notification: NotifyAssignee/NextLevel
           IncidentResolved            → slo: RecordRecovery
           PostmortemCompleted         → notification: NotifyTeam
```

### 5.2 Outbox Implementation

Write path — **cùng một DB transaction**:

```sql
BEGIN;
  INSERT INTO alert_rules (...);
  INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload)
       VALUES ('AlertRule', $id, 'AlertRuleCreated', $json);
COMMIT;
```

Relay — background goroutine (có stop/done channel theo chuẩn concurrency):

```sql
-- Poll mỗi 1s, batch 100. SKIP LOCKED cho phép chạy nhiều instance an toàn.
SELECT id, event_type, payload FROM outbox_events
WHERE status = 'pending'
ORDER BY id
LIMIT 100
FOR UPDATE SKIP LOCKED;
```

Quy tắc:
- Chỉ UPDATE `status='published'` **sau khi** mọi subscriber xử lý xong (in-process bus, synchronous dispatch).
- Subscriber phải **idempotent** — dedup theo `event id` (subscribers ghi `processed_events` hoặc dùng upsert tự nhiên).
- Event failed → `retry_count++`, backoff; quá 5 lần → `status='failed'` + metric `logmon_outbox_failed_total` + alert.
- Cleanup: xóa events `published` cũ hơn 7 ngày (cron).
- Baseline tuning: poll 1000ms / batch 100 — điều chỉnh theo metric `logmon_outbox_lag_seconds`.

**Evolution path:** GĐ 1-3 outbox + in-memory bus (1 process — đủ). Khi tách service: relay publish sang Kafka thay vì bus nội bộ (chỉ đổi adapter, không đổi domain). Tùy chọn: dùng Watermill Forwarder + watermill-sql thay vì tự viết relay nếu đã quyết định dùng Watermill cho event bus.

---

## 6. Resilience Patterns (shared/resilience)

| Pattern | Implementation | Cấu hình mặc định |
|---------|----------------|--------------------|
| Circuit breaker | `sony/gobreaker/v2` (generics) | Open khi ≥5 requests và ≥50% failure; half-open sau 10s, thử 3 requests |
| Retry + jitter | custom `WithRetry` | 3 attempts, initial 100ms, ×2, max 5s, jitter ±25%; chỉ retry 502/503/504 + network errors |
| Timeout | per call type | HTTP service-to-service 5s · DB query 3s · Redis 500ms · ES search 10s · webhook ngoài 10s · graceful shutdown 30s |
| Fallback | cache-aside + stale cache | Thứ tự: cache → nguồn chính (CB-protected) → stale cache (log warning) → error |
| Rate limiting | `redis_rate/v10` GCRA | Per-IP (GĐ1), per-workspace (GĐ3); fail-open khi Redis down (log + metric) |

Metrics bắt buộc: `logmon_circuit_breaker_state{service,state}`, `logmon_retry_attempts_total`, `logmon_fallback_usage_total{fallback_type}`.

---

## 7. Cấu Trúc Thư Mục BC (mẫu chuẩn)

```
internal/alerting/
├── domain/
│   ├── alert_rule.go          ← Aggregate Root + business rules
│   ├── alert_instance.go      ← Entity
│   ├── silence.go             ← Entity
│   ├── severity.go            ← Value Object
│   ├── events.go              ← Domain Events
│   └── errors.go              ← ErrRuleNotFound, ValidationError...
├── app/
│   ├── command/               ← create_rule.go, update_rule.go, acknowledge.go,
│   │                            silence.go, ingest_webhook.go
│   └── query/                 ← active_alerts.go, alert_history.go, rule_detail.go
├── ports/
│   ├── repository.go          ← interfaces nhỏ: RuleFinder, RuleSaver (ISP)
│   ├── rule_syncer.go         ← RuleSyncer interface (render→validate→write→reload)
│   ├── event_publisher.go
│   └── read_model.go
└── adapters/
    ├── http/handler.go        ← Gin handlers + request validation
    ├── postgres/              ← repo.go, read_model.go, reconstruct.go
    ├── promfile/syncer.go     ← rule file renderer + promtool + reload (ADR-024)
    └── alertmanager/client.go ← silence API v2 client
```

Quy ước Go chi tiết (naming, error handling, concurrency, testing): theo `CLAUDE.md` + [11-coding-testing-standards.md](11-coding-testing-standards.md).
