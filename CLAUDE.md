# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

LogMon là nền tảng observability (logging + monitoring) cho Go microservices. Backend áp dụng **2 mô hình kiến trúc** tùy theo complexity của domain:

- **Clean Architecture** — cho `identity` (auth/users/workspaces/RBAC) và `notification` (CRUD-like, domain đơn giản). *Repo hiện có `internal/user/` → sẽ đổi thành `identity/`; `order` cũ là demo ở `examples/demo-order/`, không phải BC platform — ADR-029.*
- **Clean Architecture + DDD + CQRS** — cho `alerting`, `slo`, `logpipeline` (business logic phức tạp)

Tài liệu chi tiết: `doc_v2/` — 18 file thiết kế (00-tong-quan → 17-ai-incident-automation), là source of truth.

## Tech Stack

- **Backend**: Go 1.26+, Gin, zerolog, prometheus/client_golang, pgx/v5, go-redis
- **Frontend**: Next.js 16+, TypeScript, TailwindCSS, shadcn/ui
- **Metrics**: Prometheus (PULL) + Thanos (long-term, Mode B) → Alertmanager → Slack/Email/PagerDuty
- **Logs**: zerolog → OTel Collector (agent → gateway) → Elasticsearch (data streams + ILM); Kafka buffer chỉ ở Mode B (ADR-018)
- **Traces**: OpenTelemetry SDK → OTel Collector (tail sampling) → Jaeger v2 (storage Elasticsearch)
- **Visualization**: Grafana 13.1+ (metrics/logs/traces; Git Sync GA), Next.js dashboard (admin UI)
- **Infrastructure**: Docker Compose (dev), Kubernetes (prod), Nginx reverse proxy

## Build & Run Commands

> Vòng lặp local thống nhất qua **root `Makefile`** (`make help` để xem hết).
> Stack nặng (ES/Grafana/Prometheus/OTel) nằm sau profile `observability`;
> demo workload sau profile `demo`. Migrations dùng **golang-migrate** (không
> còn initdb) — service `migrate` chạy one-shot khi `up`.

```bash
# Local dev (khuyến nghị — root Makefile)
make doctor          # kiểm tra toolchain (docker/compose/go/pnpm/chrome)
make up              # stack nhẹ: postgres + migrate + userservice
make up-full         # + observability (ES/Grafana/Prometheus/Alertmanager/OTel)
make up-demo         # + demo workload (demo-order + loadgen)
make seed            # nạp dữ liệu demo (idempotent)
make dev             # DB + hướng dẫn hot-reload; make dev-be / make dev-fe
make migrate         # áp migrations (golang-migrate up); make migrate-down để rollback
make test            # unit test BE (go -race) + FE (vitest)
make e2e             # full-stack Playwright (tự dựng + teardown)
make down            # dừng (down-v để xoá volume)

# Trực tiếp (khi cần)
cd backend && go build ./... && go test -race ./... && golangci-lint run
cd frontend && pnpm install && pnpm build && pnpm test
```

## Architecture Rules (MUST FOLLOW)

### Layer Direction (strict, one-way)

```
adapters → ports ← app → domain
```

- **domain/** không import gì ngoài Go standard library
- **app/** chỉ import `domain` và `ports`
- **ports/** chỉ chứa interfaces, không chứa implementation
- **adapters/** implement interfaces từ `ports`
- **KHÔNG** cross-BC imports — BCs giao tiếp qua domain events hoặc shared kernel

### Bounded Contexts

| BC | Pattern | Lý do |
|----|---------|-------|
| `internal/identity/` | Clean Architecture | Auth + users + workspaces + RBAC — CRUD + policy, domain đơn giản (repo hiện là `internal/user/`, sẽ đổi tên — ADR-029) |
| `internal/alerting/` | Clean Arch + DDD + CQRS | Business rules phức tạp: threshold, inhibition, routing, escalation |
| `internal/slo/` | Clean Arch + DDD + CQRS | Error budget calculation, burn rate, compliance tracking |
| `internal/logpipeline/` | Clean Arch + DDD + CQRS | Mode switching, DLQ retry, ILM policy management |
| `internal/incident/` | Clean Arch + DDD + CQRS | State machine 7 trạng thái, severity SEV1-4, MTTA/MTTR, on-call rotation, escalation, postmortem (GĐ3) |
| `internal/notification/` | Clean Architecture | Hub đa kênh (Slack/Email/PagerDuty/Teams/webhook/in-app), template, retry/queue — domain đơn giản (GĐ3) |
| `internal/shared/` | Shared Kernel | Auth, errors, logger, metrics middleware, event bus |

> **GĐ5 (AI incident automation):** lớp AI là **service Python độc lập** (HolmesGPT + WeKnora), **ngoài Go core** — tích hợp qua MCP/webhook/event, KHÔNG cross-BC import, KHÔNG vi phạm layer direction. Xem `doc_v2/17-ai-incident-automation.md` + ADR-032. `order` cũ → `examples/demo-order/` (demo, không phải BC).

### Clean Architecture Layers (identity, notification)

```
domain/       — entities, value objects, domain errors
app/          — use cases (application services)
ports/        — interfaces (repository, cache, external services)
adapters/     — implementations (http handlers, postgres repos, redis cache)
```

### DDD + CQRS Layers (alerting, slo, logpipeline)

```
domain/       — aggregates, value objects, domain events, domain services
app/command/  — write side: state-changing use cases
app/query/    — read side: optimized read models, có thể dùng cache/materialized views
ports/        — interfaces + EventPublisher + ReadModel interfaces
adapters/     — implementations + infrastructure adapters (Prometheus, Slack, Kafka, ES)
```

## Go Style Guide

> Chi tiết + code examples: `doc_v2/11-coding-testing-standards.md`
> Tham khảo: [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md), [SOLID Go — Dave Cheney](https://dave.cheney.net/2016/08/20/solid-go-design), [OWASP Go-SCP](https://owasp.org/www-project-go-secure-coding-practices-guide/)

### Error Handling
- Wrap errors with concise context: `fmt.Errorf("get alert: %w", err)` (KHÔNG dùng "failed to")
- `%w` khi caller cần match, `%v` khi ẩn implementation (adapter boundary)
- Handle once: log HOẶC return, KHÔNG làm cả hai
- Domain errors: `var ErrAlertNotFound = errors.New(...)` + custom `ValidationError` type
- KHÔNG panic trong production — chỉ `errors.New()` / `fmt.Errorf()`

### Interface Design (SOLID)
- **Accept interfaces, return structs** (ISP)
- Small, focused interfaces: `AlertFinder` (1 method) thay vì `AlertRepository` (6 methods)
- Verify compliance tại compile time: `var _ ports.Notifier = (*SlackNotifier)(nil)`
- Domain defines interfaces (DIP): `ports/` chứa interfaces, `adapters/` implement
- KHÔNG dùng pointer to interface, KHÔNG embed `sync.Mutex`

### Code Organization
- Package names: lowercase, no `common`/`util`/`helpers`
- Imports: 2 groups (stdlib | third-party), separated by blank line
- Function order: type → constructor → methods → utilities
- Early return / guard clauses, avoid deep nesting
- Functional options cho service configuration
- `run()` pattern: `main()` chỉ gọi `run()`, exit once

### Concurrency
- Mọi goroutine PHẢI có stop signal + wait mechanism (stop/done channels hoặc WaitGroup)
- Copy slices/maps tại API boundaries
- Mutex: named field (KHÔNG embed), zero value is ready
- Channel sizes: chỉ 0 hoặc 1
- KHÔNG goroutines trong `init()`

### Naming
- Exported error vars: `ErrAlertNotFound`
- Unexported globals: `_defaultScrapeInterval`
- Error types: suffix `Error` → `ValidationError`
- Enums start at 1 (`iota + 1`)
- Tests: `tests` slice, `tt` case, `give`/`want` prefix

### General
- Logging: chỉ dùng zerolog wrapper, KHÔNG `log.Println` / `fmt.Print`
- Tests: table-driven, `testify/require` cho setup (KHÔNG `assert`), inject dependencies (KHÔNG mutable globals)
- `context.Context` as first param cho mọi function có side effect
- Metrics naming: `snake_case`, prefix `logmon_`, Counter suffix `_total`
- KHÔNG dùng high-cardinality labels: `user_id`, `request_id`, `trace_id`
- `strconv` over `fmt` cho primitive conversion; specify container capacity

## Security (OWASP)

- **Input validation**: `go-playground/validator/v10` cho tất cả request structs
- **Parameterized queries**: LUÔN dùng `$1, $2` (pgx), KHÔNG string concatenation
- **Auth**: argon2id cho passwords (ADR-022; bcrypt là legacy — migrate ở GĐ2), JWT `HttpOnly + Secure + SameSite` + refresh rotation (ADR-023)
- **Errors to users**: generic messages only, log chi tiết internally
- **HTTP headers**: HSTS, X-Content-Type-Options, X-Frame-Options trên mọi response
- **TLS**: MinVersion TLS 1.2, `InsecureSkipVerify: false` ALWAYS
- **Secrets**: env vars / secrets manager, KHÔNG hardcode, KHÔNG commit `.env`
- **Random**: `crypto/rand` cho tokens, KHÔNG `math/rand`
- **Anti-patterns**: KHÔNG `unsafe` package, KHÔNG `text/template` cho HTML, KHÔNG `log.Fatal` trong handlers

## Domain Events (DDD BCs only)

Cross-BC communication qua domain events thay vì direct imports:
```
AlertFired → NotificationService.Send()
AlertFired → SLOService.RecordFailure()
AlertResolved → NotificationService.SendRecovery()
BudgetExhausted → AlertingService.CreateCriticalAlert()
PipelineModeChanged → AlertingService.UpdatePipelineAlerts()
```

## Key Documentation

- `doc_v2/` — Tài liệu thiết kế chi tiết (source of truth): 00-tong-quan · 01-kien-truc-tong-the · 02-backend-architecture · 03-logs-pipeline · 04-metrics-tracing · 05-alerting-slo · 06-incident-notification · 07-api-specification · 08-database-schema · 09-security · 10-deployment-operations · 11-coding-testing-standards · 12-roadmap · 13-adr · 14-frontend-architecture · 15-devsecops-cicd · 16-iac-runbooks · 17-ai-incident-automation
- `README.md` — Tổng quan + quick start cho thành viên mới

## Frontend Design Skills

- **`taste-skill`** (vendored, MIT — `.claude/skills/taste-skill/`): anti-slop frontend taste cho
  **landing / marketing / portfolio / redesign**. Theo phạm vi gốc của skill, **KHÔNG** dùng cho
  dashboards / data tables / multi-step product UI. Chỉ invoke khi làm việc thiết kế FE (file 87KB,
  không để always-on). Provenance: `.claude/skills/taste-skill/SOURCE.md`.
- **Admin dashboard data-table** (phần chính của LogMon): ưu tiên `ecc:dashboard-builder` +
  `ecc:design-system` + shadcn/ui; review bằng `ecc:react-reviewer` + `ecc:a11y-architect`.

## Language

Project documentation is in Vietnamese. Respond and interact in Vietnamese.
