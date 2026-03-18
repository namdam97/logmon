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

## Style Guide

- Error wrapping: `fmt.Errorf("doing X: %w", err)`
- Logging: chỉ dùng zerolog wrapper từ `shared/logger/`, KHÔNG dùng `log.Println` / `fmt.Print`
- Tests: table-driven, dùng `testify/require` (không `assert`) cho setup steps
- No global state, dependency injection qua constructor
- Context propagation: mọi function có side effect nhận `context.Context`
- Interface định nghĩa ở `ports/` (nơi sử dụng), không ở nơi implement
- Metrics naming: `snake_case`, prefix `logmon_`, Counter suffix `_total`
- KHÔNG dùng high-cardinality labels: `user_id`, `request_id`, `trace_id`

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
