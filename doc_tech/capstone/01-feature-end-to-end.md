# Capstone — Xây 1 feature xuyên suốt mọi tầng

> Capstone-1 · "Vertical slice": đi 1 tính năng từ domain → API → FE → observability → test → deploy, **map mỗi bước về module + file thật** trong LogMon. Đây là bài ráp tất cả lại.

## Mục tiêu

Bạn đã học rời từng tầng. Capstone này dạy **luồng làm việc thật của một kỹ sư**: thêm một feature mỏng-nhưng-đầy-đủ (vertical slice) chạm mọi tầng, đúng layer direction `adapters → ports ← app → domain` và bám doc_v2.

**Feature ví dụ (planned, chưa có code — bạn sẽ tự dựng):** *"Maintenance window cho Alert Rule"* — trong khung giờ bảo trì, rule **không fire** (tránh báo động giả khi deploy). Đây là concept mới, không trùng `enabled` sẵn có, nên buộc đi qua đủ tầng.

> Nguyên tắc xuyên suốt: **viết test trước (TDD)**, **domain không import hạ tầng**, **cross-BC qua event không import trực tiếp**, **không nuốt lỗi**, **đo được** (metric/log/trace).

## Bản đồ 12 bước (mỗi bước → module học + file neo)

| # | Tầng | Việc làm | Module | File neo (thật) |
|---|------|----------|--------|-----------------|
| 1 | **domain** | Thêm VO `MaintenanceWindow` (start, end, cron?) + method `AlertRule.IsInMaintenance(now)` (invariant: end > start). Bất biến, copy-on-write. | [ARCH-2](../architecture/02-ddd-bounded-contexts.md) · [BE-1](../backend-go/01-go-production.md) | `backend/internal/alerting/domain/rule.go`, `.../value objects` |
| 2 | **domain event** | Định nghĩa `AlertRuleMaintenanceScheduled` trong `events.go`. | [ARCH-3](../architecture/03-cqrs-event-driven.md) | `backend/internal/alerting/domain/events.go` |
| 3 | **ports** | Mở rộng `RuleRepository` (đã có Update) — không cần method mới nếu lưu trong rule. Interface nhỏ (ISP). | [ARCH-1](../architecture/01-clean-architecture.md) | `backend/internal/alerting/ports/` |
| 4 | **app/command** | Use case `ScheduleMaintenance` — load rule, gọi domain, lưu **trong cùng TX** + publish event qua outbox (`WithinTx`). | [ARCH-3](../architecture/03-cqrs-event-driven.md) | `backend/internal/alerting/app/` (mẫu: `create_rule.go`) |
| 5 | **migration** | `000007_*.up.sql`: thêm cột `maintenance` JSONB (hoặc bảng riêng). Có `.down.sql`. | [BE-3](../backend-go/03-postgres-pgx-migrations.md) | `backend/migrations/` ([ERD](../backend-go/03-postgres-pgx-migrations.md)) |
| 6 | **adapter postgres** | Cập nhật repo: map cột mới, parameterized query `$1,$2`. | [BE-3](../backend-go/03-postgres-pgx-migrations.md) | `backend/internal/alerting/adapters/postgres/` |
| 7 | **adapter http + API** | `POST /api/v1/alert-rules/{id}/maintenance` — bind + validate (validator/v10), trả `httpx.Envelope`. Map domain error → HTTP. | [BE-2](../backend-go/02-gin-http-api.md) · [BE-6](../backend-go/06-api-design-openapi.md) | `backend/internal/alerting/adapters/http/handler.go`, `shared/httpx` |
| 8 | **consumer / sync** | Subscribe event → khi tới giờ bảo trì, syncer **không** đẩy rule sang Prometheus (hoặc thêm `inhibit`). Idempotent. | [ARCH-3](../architecture/03-cqrs-event-driven.md) · [OBS-1](../observability/01-metrics-prometheus.md) | `backend/internal/alerting/adapters/` (prom syncer) |
| 9 | **frontend** | Cột/toggle "Maintenance" trên bảng rule + form chọn khung giờ; gọi API; optimistic update. | [FE-1](../frontend/01-nextjs-typescript.md) · [FE-2](../frontend/02-tailwind-shadcn.md) | `frontend/app/(dashboard)`, `frontend/components/ui` |
| 10 | **observability** | Metric `logmon_alert_rule_maintenance_active` (Gauge), log có cấu trúc khi vào/ra bảo trì, span trace cho use case. | [OBS-1](../observability/01-metrics-prometheus.md) · [OBS-2](../observability/02-logs-otel-elasticsearch.md) · [OBS-3](../observability/03-traces-opentelemetry-jaeger.md) | `backend/internal/shared/{metrics,logger,tracing}` |
| 11 | **test** | Unit (domain `IsInMaintenance` table-driven), integration (repo + API qua testcontainers), e2e (Playwright bật/tắt). Coverage ≥80%. | [TEST-1](../backend-go/05-testing-strategy.md) | `*_test.go`, `frontend/e2e` |
| 12 | **CI + deploy** | Pipeline xanh (lint/test/scan), migration chạy lúc rollout, kiểm probe. | [DSO](../devsecops/02-ci-cd-supply-chain.md) · [K8S-2](../kubernetes/02-deploying-logmon.md) | `.github/workflows/ci.yml`, `infra/` |

## Luồng dữ liệu (ráp lại các sơ đồ đã học)

```
Client → [Gin middleware chain] → handler (validate) → app.ScheduleMaintenance
   → domain.AlertRule.ScheduleMaintenance() (invariant)
   → repo.Update + outbox.Publish(event)  [CÙNG 1 TX]
   → outbox relay → bus → prom-syncer (cập nhật/inhibit rule)
   → metric+log+trace phát ra suốt đường đi
```
Đối chiếu: [Gin lifecycle](../diagrams/gin-request-lifecycle.png) · [Clean Arch](../diagrams/clean-arch-layers.png) · [CQRS/Outbox (BC map)](../../doc_v2/diagrams/logmon_bc_map.png) · [3 trụ cột](../diagrams/observability-3-pillars.png).

## Thứ tự làm (TDD, từ trong ra ngoài)

1. **RED**: viết test domain `IsInMaintenance` → fail.
2. **GREEN**: implement domain tối thiểu → pass. Refactor.
3. Lên dần: app (fake repo/bus) → migration+repo (integration) → http (integration) → FE → observability.
4. Mỗi tầng có test trước khi sang tầng ngoài. **Không** viết handler trước domain.

## Definition of Done (checklist)

- [ ] `domain/` mới **không** import gì ngoài stdlib (+ shared/errors nếu cần) — chạy `golangci-lint`.
- [ ] Event phát **trong cùng TX** với thay đổi state (outbox), consumer **idempotent**.
- [ ] API trả `httpx.Envelope` nhất quán; domain error → HTTP đúng, **không leak** nội bộ.
- [ ] Migration có cả `.up`/`.down`; `make migrate` và `make migrate-down` chạy sạch.
- [ ] Có **metric + log + trace** cho hành vi mới (đo được, debug được).
- [ ] Test 3 tầng (unit/integration/e2e), coverage ≥ 80%, `go test -race` xanh.
- [ ] FE cập nhật, có e2e Playwright cho flow.
- [ ] CI xanh toàn bộ gate (lint/test/govulncheck/gitleaks/Trivy).
- [ ] Bám doc_v2: nếu feature lệch thiết kế → cập nhật doc_v2 trước (doc_v2 là source of truth).

## Biến thể để luyện thêm

- **Read-heavy slice**: `GET /api/v1/alert-rules/{id}/history` (alert_instances của rule) → luyện **CQRS read side** + pagination ([BE-6](../backend-go/06-api-design-openapi.md)).
- **Cross-BC slice**: `BudgetExhausted` (slo) → tạo critical alert (alerting) → page (notification) → mở incident → AI điều tra. Chạm [SLO](../observability/04-grafana-slo.md) · [INC-1](../incident/01-incident-management.md) · [NOT-1](../notification/01-notification-hub.md) · [AI-1](../ai/01-ai-incident-automation.md).

> Hoàn thành 1 vertical slice = bạn đã "hero": hiểu vì sao mỗi tầng tồn tại và chúng ráp với nhau ra sao. Dùng `ecc:feature-dev` / `ecc:orch-add-feature` để được dẫn dắt khi làm thật.
