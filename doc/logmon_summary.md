# LogMon — Executive Summary

> **Nắm bắt nhanh toàn bộ hệ thống trong 5 phút.**
> Chi tiết đầy đủ: [`logmon.md`](logmon.md) (4400+ dòng, 27 sections)

---

## LogMon là gì?

Self-hosted **observability platform** cho Go microservices, cung cấp **3 trụ cột observability** + operational tooling:

```
Metrics (Prometheus + Thanos)  ──┐
Logs    (ELK pipeline)         ──┼──→  Grafana + Next.js Dashboard
Traces  (OpenTelemetry + Jaeger) ┘          │
                                            ▼
                                   Alerting → Incident Management → Postmortem
```

---

## Tech Stack (1 dòng mỗi component)

| Layer | Stack |
|-------|-------|
| Backend | Go 1.22+, Gin, zerolog, pgx/v5, go-redis |
| Frontend | Next.js 14+, TypeScript, TailwindCSS, shadcn/ui |
| Metrics | Prometheus (15d local) → Thanos → MinIO/S3 (1 year) |
| Logs | Filebeat → [Kafka buffer] → Logstash → Elasticsearch |
| Traces | OTel SDK → OTel Collector → Jaeger (ES backend) |
| Alerting | Alertmanager → Slack / Email / PagerDuty / Teams / Webhooks |
| Visualization | Grafana 10.4+ (metrics + logs + traces, single pane) |
| Database | PostgreSQL (business data), Redis (cache, rate limiting) |
| Storage | MinIO / S3 (long-term metrics, ES snapshots, backups) |
| Infra | Docker Compose (dev) / Kubernetes (prod), Nginx reverse proxy |

---

## Kiến trúc Backend — 8 Bounded Contexts

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Clean Architecture                           │
│  ┌──────────┐  ┌──────────┐  ┌────────────────┐                   │
│  │  order/   │  │  user/   │  │  notification/ │   ← CRUD-like    │
│  └──────────┘  └──────────┘  └────────────────┘                   │
│                                                                     │
│                  Clean Architecture + DDD + CQRS                    │
│  ┌──────────┐  ┌──────────┐  ┌──────────────┐  ┌──────────────┐  │
│  │ alerting/ │  │   slo/   │  │ logpipeline/ │  │  incident/   │  │
│  └──────────┘  └──────────┘  └──────────────┘  └──────────────┘  │
│        │              │              │                │             │
│        └──────────────┴──────────────┴────────────────┘             │
│                     Domain Events (Outbox Pattern)                  │
│                                                                     │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │  shared/  — auth, tracing, logger, metrics, resilience,     │  │
│  │             middleware (recovery, logging, RBAC, workspace), │  │
│  │             eventbus (outbox)                                │  │
│  └──────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
```

**Layer direction (strict):** `adapters → ports ← app → domain`

| BC | Vai trò | Key Concepts |
|----|---------|--------------|
| **alerting/** | Alert rules, firing, silence, inhibition | Aggregate: AlertRule, Events: AlertFired/Resolved |
| **slo/** | Error budget, burn rate, compliance tracking | Aggregate: SLO, Events: BudgetExhausted |
| **logpipeline/** | Mode switching (A/B), DLQ retry, ILM policy | Aggregate: Pipeline, Events: ModeChanged |
| **incident/** | Lifecycle: create → triage → assign → resolve → postmortem | Aggregate: Incident, On-call rotation, MTTR |
| **notification/** | Multi-channel delivery, templates, retry queue | Slack, Email, PagerDuty, Teams, webhooks |
| **order/** | Demo CRUD service (domain đơn giản) | Entity: Order |
| **user/** | Auth, user management | Entity: User, bcrypt + JWT |
| **shared/** | Cross-cutting concerns | Auth, tracing, resilience, eventbus |

---

## 4 Luồng Dữ Liệu

### Metrics (PULL)
```
Go Services → /metrics → Prometheus (scrape 15s) → Thanos → MinIO (long-term)
                                ↓
                         Alert rules → Alertmanager → Slack/Email
                                ↓
                         Grafana (PromQL dashboards)
```

### Logs (PUSH)
```
Go Services → stdout JSON → Filebeat → [Kafka buffer] → Logstash → Elasticsearch
                                                                         ↓
                                                                    ILM: hot(7d) → warm(30d) → cold(S3) → delete(180d)
                                                                         ↓
                                                                    Grafana + Log Search API
```

### Traces
```
Go Services → OTel SDK → OTel Collector → Jaeger (ES backend)
                (W3C traceparent)    (tail sampling: 100% errors, 10% normal)
                                                       ↓
                                                  Grafana (trace → log correlation via trace_id)
```

### Alerts → Incidents
```
Prometheus rule firing → Alertmanager → Notification Hub → Slack/PagerDuty
        ↓                                                         
 AlertFired event → Auto-create Incident (if critical > 5m)
        ↓
 Incident lifecycle: open → triage → assign → mitigate → resolve → postmortem
        ↓
 MTTR/MTTA metrics → SLO impact tracking
```

---

## Deployment: 2 Modes

| | Mode A (Dev/Small) | Mode B (Production) |
|---|---|---|
| Command | `docker compose up` | `docker compose --profile scale up` |
| Log pipeline | Filebeat → Logstash → ES | Filebeat → **Kafka** → Logstash → ES |
| Long-term metrics | Không | **Thanos → MinIO** |
| Traces | Jaeger (in-memory) | Jaeger (**ES backend**) |
| RAM | ~8 GB | ~32 GB |
| Phù hợp | Dev, staging, < 5K logs/s | Production, > 10K logs/s |

---

## Multi-tenancy & RBAC

```
Platform
  └── Workspace "backend-team"
  │     ├── Members: admin, editor, viewer
  │     ├── Alert Rules (workspace-scoped)
  │     ├── SLOs, Incidents, Logs (isolated)
  │     └── Notification Channels (self-configured)
  └── Workspace "infra-team"
        └── ...
```

4 roles: **viewer** (read) → **editor** (CRUD) → **admin** (manage workspace) → **platform_admin** (all workspaces)

---

## API Overview (~80 endpoints)

| Group | Base Path | Key Endpoints |
|-------|-----------|---------------|
| Auth | `/api/v1/auth/` | login, logout, refresh, me |
| Alerts | `/api/v1/alerts/` | rules CRUD, active, acknowledge, silence, history |
| SLOs | `/api/v1/slos/` | CRUD, budget, burn-rate, compliance |
| Pipeline | `/api/v1/pipeline/` | status, mode switch, DLQ retry, ILM, indices |
| Log Search | `/api/v1/logs/` | search (POST), tail (SSE), trace/:id, stats |
| Incidents | `/api/v1/incidents/` | CRUD, triage, assign, status, timeline, postmortem |
| On-call | `/api/v1/oncall/` | current, schedule, override |
| Notifications | `/api/v1/notifications/` | channels CRUD, templates, test, history |
| Reports | `/api/v1/reports/` | schedules, generate, history, download |
| Export | `/api/v1/export/` | logs/metrics export (async jobs) |
| Topology | `/api/v1/topology` | service dependency graph + health |
| Billing | `/api/v1/billing/usage` | per-workspace ingestion/cost |
| Workspaces | `/api/v1/workspaces/` | CRUD, members, roles |

---

## Database: ~25 Tables

```
Shared:      workspaces, users, workspace_members, audit_logs, outbox_events
Alerting:    alert_rules, alert_instances, silences
SLO:         slos, slo_snapshots
LogPipeline: pipeline_configs, dlq_entries
Incident:    incidents, incident_timeline, postmortems, postmortem_actions,
             oncall_schedules, oncall_overrides
Notification: notification_channels, notification_history
Reports:     report_schedules, report_history, export_jobs
Order:       orders
```

---

## Resilience Patterns

| Pattern | Thư viện / Approach | Mô tả |
|---------|---------------------|-------|
| **Circuit Breaker** | sony/gobreaker | Open khi ≥50% failure, half-open sau 10s |
| **Retry + Jitter** | Custom (shared/resilience) | 3 attempts, exponential backoff, ±25% jitter |
| **Timeout** | Per call type | Service-to-service: 5s, DB: 3s, Redis: 500ms |
| **Rate Limiting** | Redis sliding window | Per-workspace quotas, 5 layers |
| **Outbox Pattern** | PostgreSQL + relay goroutine | At-least-once event delivery, no event loss |
| **Log Sampling** | OTel tail sampling | 100% errors, 100% slow (>1s), 10% normal |

---

## Key Architecture Decisions (17 ADRs)

| # | Decision | Lý do |
|---|----------|-------|
| 001 | Clean Arch + DDD + CQRS cho complex BCs | Monitoring read:write ~100:1, domain logic phức tạp |
| 003 | ELK thay vì Loki | Full-text search bất kỳ field, aggregation/analytics |
| 010 | OpenTelemetry cho tracing | Industry standard, vendor-neutral, trace-to-log correlation |
| 011 | Thanos cho long-term metrics | SLO cần 30-90d data, Prometheus chỉ 15d |
| 012 | Multi-tenancy via Workspace | Shared infra, data isolation, self-service teams |
| 014 | Incident Management BC | Full SRE lifecycle: alert → incident → postmortem |
| 015 | Notification Hub (extensible) | Plugin architecture: Slack, PagerDuty, Teams, webhooks |
| 016 | Outbox Pattern cho event bus | At-least-once delivery, no event loss khi crash |
| 017 | Object Storage tiering | Cost giảm 5-10x cho data > 7 ngày |

> Xem đầy đủ 17 ADRs trong [`logmon.md`](logmon.md) Section 10 + 19.

---

## Cost Estimates

| Scale | Services | RAM | Disk | Monthly Cost (VPS) |
|-------|----------|-----|------|--------------------|
| **Small** | 2-5 | 8 GB | 270 GB | ~$40 |
| **Medium** | 5-20 | 32 GB | 1.5 TB | ~$150-250 |
| **Large** | 20-100 | 80 GB | 6 TB | ~$500-1000 |

> So sánh: Datadog ~$300/mo cho Medium, New Relic free tier đủ cho Small.

---

## Personas

| Persona | Dashboard chính | Hành động chính |
|---------|-----------------|-----------------|
| **Developer** | Service Overview + Logs Explorer + Traces | Debug errors, trace requests, search logs |
| **DevOps** | Infrastructure + Pipeline Status | Monitor CPU/RAM/disk, manage pipeline mode |
| **SRE** | SLO Dashboard + Incidents + Alerting | Track error budget, manage incidents, on-call |
| **Manager** | Weekly Reports + Cost Dashboard | SLO compliance, MTTR trends, cost tracking |

---

## DevOps Pipeline

```
Code → Git Push → CI (test + lint + build) → Container Registry
                                                    ↓
                                        Staging (auto-deploy)
                                                    ↓
                                        Production (manual approve)
                                                    ↓
                                        Post-deploy verify (health + metrics + logs)
```

---

## Roadmap Phases

| Phase | Scope | Infrastructure |
|-------|-------|----------------|
| **1. MVP** | 1 VPS, Docker Compose, Mode A | Ubuntu + Docker + Nginx |
| **2. CI/CD** | Auto test & deploy | + GitHub Actions |
| **3. Multi-env** | Staging + Production (Mode B) | + Kafka, Thanos, MinIO |
| **4. Scale** | Auto-scaling, HA | Kubernetes (EKS/AKS/GKE) |

---

## Remaining Future Work

| Feature | Priority | Note |
|---------|----------|------|
| SQL-based log query (ClickHouse) | Low | ES full-text đủ cho 95% use cases |
| AI/ML anomaly detection | Low | Cần historical data + ML infrastructure |

---

## Làm Chủ LogMon = Level Up Như Thế Nào?

LogMon không chỉ là 1 dự án — đây là **bản đồ skill tree** cover gần như toàn bộ năng lực cần thiết để đi từ Mid-level lên Senior/Staff Engineer. Mỗi component bạn implement = 1 skill thực chiến có thể demonstrate trong interview hoặc áp dụng ngay tại công ty.

### 1. Technical Skills — "Depth"

| Skill | LogMon component dạy bạn | Level tương đương |
|-------|--------------------------|-------------------|
| **Go Backend thuần thục** | 8 BCs, Gin handlers, pgx, go-redis, zerolog | Senior Go Developer |
| **Clean Architecture** | Layer direction, ports/adapters, dependency inversion | Ai cũng nói "clean code" nhưng ít người build xong 1 hệ thống thật |
| **DDD (Domain-Driven Design)** | Aggregates, Value Objects, Domain Events, Bounded Contexts | Staff-level design skill — phân biệt bạn với developer chỉ biết CRUD |
| **CQRS** | Command/Query split, read models, materialized views | Giải quyết bài toán read:write ratio cực lệch (monitoring, analytics, reporting) |
| **Event-Driven Architecture** | Outbox pattern, domain events, cross-BC communication | Foundation cho microservices communication — Kafka, NATS, RabbitMQ đều dùng pattern này |
| **Distributed Tracing** | OpenTelemetry SDK, W3C Trace Context, span propagation | Kỹ năng debug production mà 90% developer thiếu |
| **Database Design** | PostgreSQL schema, indexes, ERD, ILM lifecycle | Từ "biết viết query" lên "biết thiết kế schema cho scale" |
| **Elasticsearch** | Full-text search, index management, cluster sizing | Niche skill, demand cao, ít người biết sâu |
| **Message Queue** | Kafka (producer, consumer, partitions, DLQ, replay) | Bắt buộc cho mọi hệ thống distributed |
| **Caching Strategy** | Redis (cache aside, rate limiting, session, read model cache) | Biết khi nào cache, cache gì, invalidate thế nào |
| **Resilience Engineering** | Circuit breaker, retry + jitter, timeout, fallback | Khác biệt giữa code chạy ở local vs code sống được trên production |
| **Security (OWASP)** | Input validation, parameterized queries, JWT, TLS, RBAC | Mỗi vulnerability bạn biết prevent = 1 sự cố production tránh được |
| **Concurrency (Go)** | Goroutine lifecycle, channels, mutex, graceful shutdown | Go's superpower — nhưng cũng là nguồn bug #1 nếu không master |

### 2. Infrastructure & DevOps Skills — "Breadth"

| Skill | LogMon component dạy bạn | Giá trị thực tế |
|-------|--------------------------|-----------------|
| **Docker & Compose** | Multi-service orchestration, profiles, healthchecks | "Nó chạy trên máy tôi" → "Nó chạy mọi nơi" |
| **Kubernetes** | Phase 4 deployment (pods, services, ingress, HPA) | Bắt buộc cho mọi Senior+ DevOps/Backend position |
| **CI/CD Pipeline** | GitHub Actions: test → lint → build → deploy → verify | Hiểu luồng code → production end-to-end |
| **Nginx** | Reverse proxy, SSL termination, routing | Infra cơ bản nhưng nhiều dev không biết configure |
| **Prometheus + Grafana** | Metric collection, PromQL, dashboard design | Standard monitoring stack — dùng ở hầu hết công ty |
| **ELK Stack** | Log pipeline, Logstash parsing, ES index management | Enterprise logging — skill chuyên sâu, salary premium |
| **Object Storage** | MinIO/S3, tiering strategy, backup/restore | Cloud-native storage pattern |

### 3. Domain Knowledge — "Chiều sâu nghiệp vụ"

| Domain | LogMon dạy bạn | Tại sao quan trọng |
|--------|----------------|---------------------|
| **Observability Engineering** | 3 pillars (metrics, logs, traces) + correlation | Bạn không chỉ viết code — bạn **hiểu code đang chạy thế nào** trên production |
| **SRE Practices (Google)** | SLO, error budget, burn rate, MTTR, incident response | Google SRE Book không còn là lý thuyết — bạn đã build hệ thống implement nó |
| **Incident Management** | Lifecycle, on-call, escalation, postmortem, action items | Kỹ năng vận hành production mà không course nào dạy bằng tự build |
| **Platform Engineering** | Multi-tenancy, RBAC, quotas, self-service API | Trend mới: platform team serve internal developer teams |
| **Cost Optimization** | Tiering, sampling, rate limiting, capacity planning | Cloud bill là bài toán thực — biết optimize = tiết kiệm hàng nghìn $/tháng |
| **Compliance & Audit** | Audit logs, data retention, immutable trail | Bắt buộc cho regulated industries (fintech, healthcare, enterprise) |

### 4. Architecture & Design Skills — "Tầm nhìn"

| Skill | LogMon dạy bạn | Level |
|-------|----------------|-------|
| **System Design** | Thiết kế hệ thống 15+ components, data flow, trade-offs | **Đây chính xác là bài System Design Interview** ở FAANG/Big Tech |
| **ADR (Architecture Decision Records)** | 17 ADRs với context, decision, consequences | Tư duy "tại sao chọn X thay vì Y" — Senior mindset |
| **Bounded Context decomposition** | 8 BCs, mỗi BC có lý do tồn tại riêng | Phân tách complexity thay vì xây monolith |
| **Trade-off Analysis** | ELK vs Loki, PULL vs PUSH, CQRS vs không, Kafka vs không | Không có "best practice" — chỉ có "best trade-off cho context này" |
| **Progressive Architecture** | Mode A → B, in-memory → outbox → Kafka → CDC | Bắt đầu đơn giản, evolve khi cần — nguyên tắc vàng |
| **API Design** | REST conventions, pagination, error format, versioning | Thiết kế API mà frontend/mobile/partner team muốn dùng |
| **Data Modeling** | 25 tables, relationships, indexes, ERD | Từ "entity" trên giấy → schema chạy production |

### 5. Career Impact — "Giá trị thị trường"

```
                        Bạn TRƯỚC LogMon          Bạn SAU LogMon
                        ──────────────────        ────────────────────────
Khi được hỏi            "Tôi biết Go, REST"      "Tôi build observability platform
System Design                                      với DDD+CQRS, 8 BCs,
Interview                                          distributed tracing, SLO tracking,
                                                   incident management"

Khi debug production    Đọc log, đoán            Trace request cross-service,
                                                   correlate traces ↔ logs ↔ metrics,
                                                   check error budget, analyze root cause

Khi thiết kế hệ thống  Copy tutorial             ADR-driven decisions,
mới                                               trade-off analysis,
                                                   progressive architecture

Khi vận hành            "Nó crash rồi,            On-call rotation, incident lifecycle,
                        ai fix?"                   MTTR tracking, blameless postmortem
```

### 6. Skill Map → Job Titles

| Job Title | LogMon skills áp dụng trực tiếp |
|-----------|--------------------------------------|
| **Senior Backend Engineer** | Go, Clean Arch, DDD, PostgreSQL, Redis, API design, testing |
| **Senior DevOps Engineer** | Docker, K8s, CI/CD, Prometheus, ELK, Nginx, backup/DR |
| **SRE (Site Reliability Engineer)** | SLO, incident management, on-call, MTTR, alerting, capacity planning |
| **Platform Engineer** | Multi-tenancy, RBAC, self-service API, rate limiting, cost control |
| **Observability Engineer** | Tracing (OTel), metrics (Prometheus), logs (ELK), Grafana, correlation |
| **Staff/Principal Engineer** | System design, ADRs, trade-off analysis, BC decomposition, event-driven architecture |

### 7. Tổng Kết: Skill Tree Progression

```
Junior                    Mid-level                 Senior                   Staff
───────                   ─────────                 ──────                   ─────
Viết CRUD API             Viết Clean Arch           Thiết kế DDD + CQRS      Decompose system
                                                                              thành Bounded Contexts

Dùng log.Println          Dùng zerolog              Build log pipeline        Design observability
                          structured logging        (ELK + Kafka)             platform (3 pillars)

Deploy bằng tay           Docker Compose            CI/CD + Staging/Prod      K8s + auto-scaling
                                                                              + capacity planning

Không có monitoring       Prometheus + Grafana      + Alerting + SLO          + Incident management
                                                    + Thanos long-term        + Postmortem culture

Code chạy rồi thôi       Viết tests                Resilience patterns       Event-driven architecture
                                                    (circuit breaker, retry)  (outbox, domain events)

Fix bug khi user report   Monitor metrics           On-call rotation          Design self-healing
                                                    + alert rules             systems + runbooks

────────────────────────────────────────────────────────────────────────────────────────────
                    LogMon covers tất cả các cột trên ↑
```

> **Bottom line:** Hoàn thành LogMon = bạn có **portfolio project mạnh hơn 99% side projects** trên GitHub. Không phải todo app, không phải blog engine — đây là **production-grade infrastructure** mà mọi công ty tech đều cần. Bạn không chỉ nói "tôi biết" — bạn **chứng minh bằng code**.

---

> **File này là summary.** Mọi chi tiết implementation, code examples, DB schema, API spec đều nằm trong [`logmon.md`](logmon.md).
