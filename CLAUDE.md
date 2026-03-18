# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

LogMon là nền tảng observability (logging + monitoring) cho Go microservices. Backend áp dụng **2 mô hình kiến trúc** tùy theo complexity của domain:

- **Clean Architecture** — cho `order` và `user` services (CRUD-like, domain đơn giản)
- **Clean Architecture + DDD + CQRS** — cho `alerting`, `slo`, `logpipeline` (business logic phức tạp)

Tài liệu chi tiết: `doc/logmon.md` (1200+ lines) + 10 Mermaid diagrams trong `doc/diagrams/`.

## Tech Stack

- **Backend**: Go 1.22+, Gin, zerolog, prometheus/client_golang, pgx/v5, go-redis
- **Frontend**: Next.js 14+, TypeScript, TailwindCSS, shadcn/ui
- **Metrics**: Prometheus (PULL) → Alertmanager → Slack/Email
- **Logs**: Filebeat → Kafka (buffer) → Logstash → Elasticsearch
- **Visualization**: Grafana 10.4+ (metrics), Next.js dashboard (admin UI)
- **Infrastructure**: Docker Compose (dev), Kubernetes (prod), Nginx reverse proxy

## Build & Run Commands

```bash
# Backend (Go)
go build ./cmd/orderservice/
go test ./...
golangci-lint run

# Frontend (Next.js)
pnpm install
pnpm build
pnpm test

# Full stack
docker compose up                      # Mode A (dev, no Kafka)
docker compose --profile scale up      # Mode B (production, with Kafka)

# Mermaid diagrams
mmdc -i doc/diagrams/<file>.mmd -o doc/diagrams/<file>.png
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
| `internal/order/` | Clean Architecture | CRUD, domain đơn giản |
| `internal/user/` | Clean Architecture | CRUD, domain đơn giản |
| `internal/alerting/` | Clean Arch + DDD + CQRS | Business rules phức tạp: threshold, inhibition, routing, escalation |
| `internal/slo/` | Clean Arch + DDD + CQRS | Error budget calculation, burn rate, compliance tracking |
| `internal/logpipeline/` | Clean Arch + DDD + CQRS | Mode switching, DLQ retry, ILM policy management |
| `internal/shared/` | Shared Kernel | Auth, errors, logger, metrics middleware, event bus |

### Clean Architecture Layers (order, user)

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

> Chi tiết + code examples: `doc/logmon.md` Section 9
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
- **Auth**: bcrypt cho passwords, JWT với `HttpOnly + Secure + SameSite` cookies
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

- `doc/logmon.md` — Complete system specification
- `doc/diagrams/*.mmd` — Mermaid architecture diagrams

## Language

Project documentation is in Vietnamese. Respond and interact in Vietnamese.
