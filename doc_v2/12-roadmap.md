# 12 — Lộ Trình Triển Khai

> v1 mô tả hệ thống đích nhưng không có đường đi khả thi (8 BC, ~80 endpoints cùng lúc). v2 chia **4 giai đoạn**, mỗi giai đoạn có Definition of Done đo được, và **map với trạng thái repo hiện tại** (2026-06-11).

---

## 0. Trạng Thái Hiện Tại (đã có trong repo)

| Hạng mục | Trạng thái |
|----------|-----------|
| `shared/` kernel: errors, logger (zerolog), httpx, metrics, middleware (recovery/logging/metrics/ratelimit), auth (JWT) | ✅ Có + tests |
| `internal/user/` (→ sẽ thành `identity/`): domain + app + postgres + http, register/login/logout | ✅ Có + tests |
| Migrations: `0001_create_users.sql` (goose-style) | ✅ |
| Frontend: Next.js + shadcn/ui — login, dashboard shell, profile, auth guard, Playwright e2e | ✅ |
| `infra/docker/docker-compose.yml`: postgres + backend + frontend | ✅ Cơ bản |
| CI, observability stack (Prometheus/ES/Grafana/OTel), alerting | ❌ Chưa có |

→ **Walking skeleton đã đứng**. GĐ 1 xây trên nền này.

---

## Giai Đoạn 1 — Observability Core (MVP)

**Mục tiêu:** Một dev nhìn được metrics + logs của services trên Grafana, nhận alert qua Slack khi service lỗi. Chưa có UI quản lý alert — rules tĩnh.

| # | Hạng mục | Ghi chú |
|---|----------|---------|
| 1.1 | CI pipeline (GH Actions): test + lint v2 + govulncheck + gitleaks + build images | Làm ĐẦU TIÊN — mọi thứ sau đi qua CI |
| 1.2 | Hoàn thiện instrument backend: metrics middleware expose `/metrics`, health/ready chuẩn | Một phần đã có |
| 1.3 | `examples/demo-order/`: service mẫu instrument đầy đủ + traffic generator (k6/script) | Nguồn telemetry để test platform |
| 1.4 | Compose: Prometheus 3.12 + exporters (node/postgres/redis) + Grafana 12.3 provisioned | |
| 1.5 | Compose: Elasticsearch 9.4 (1 node, security ON) + OTel Collector agent→gateway, data stream `logs-*` + ILM policy + index template | Pipeline logs Mode A — KHÔNG Filebeat/Logstash |
| 1.6 | Alertmanager + bộ static rules nền (05 mục 3) + Slack receiver + **Watchdog → healthchecks.io** | Meta-monitoring ngay từ GĐ1 |
| 1.7 | Grafana dashboards: service-overview, logs-explorer, infrastructure, pipeline-health | JSON as-code |
| 1.8 | Nginx/Caddy + TLS, compose.prod.yaml, deploy staging lên VPS | |

**Definition of Done GĐ1:**
- [ ] `docker compose up -d` từ máy sạch → toàn stack healthy ≤ 5 phút
- [ ] Log từ demo-order xuất hiện trong Grafana ≤ 30s sau khi emit
- [ ] Kill demo-order → Slack nhận alert ServiceDown ≤ 2 phút; restore → resolved notification
- [ ] Tắt Prometheus → healthchecks.io báo deadman ≤ 15 phút
- [ ] CI xanh; coverage shared+identity ≥ 80%

---

## Giai Đoạn 2 — Alerting Platform + Tracing

**Mục tiêu:** SRE quản lý alert rules qua UI (không SSH); debug bằng traces với correlation đầy đủ; search logs qua API.

| # | Hạng mục |
|---|----------|
| 2.1 | Outbox infrastructure (`outbox_events` + relay + in-process bus) — nền cho mọi BC sau |
| 2.2 | `alerting/` BC: rule CRUD + PromQL validation + **rule sync pipeline** (render → promtool → atomic write → reload → verify) |
| 2.3 | Webhook receiver (Alertmanager → LogMon): track instances, fingerprint idempotency |
| 2.4 | Ack + Silence (sync Alertmanager API v2) + alert history |
| 2.5 | Auth nâng cấp: refresh token rotation + reuse detection + CSRF middleware (ADR-023); migration bcrypt→argon2id |
| 2.6 | Tracing: OTel SDK vào mọi service (otelgin/otelpgx/redisotel) + tail sampling gateway + **Jaeger v2** + spanmetrics connector + exemplars |
| 2.7 | zerolog hook inject trace_id/span_id; Grafana derived fields (logs↔traces↔metrics links) |
| 2.8 | Log Search API: search + tail SSE + by-trace + stats |
| 2.9 | Frontend: alert rules CRUD, active alerts, alert history, log viewer |

**DoD GĐ2:**
- [ ] Tạo alert rule qua UI → Prometheus evaluate trong ≤ 30s (sync + reload tự động); rule PromQL sai bị chặn với lỗi rõ ràng
- [ ] Alert fire → instance xuất hiện trong UI ≤ 10s sau Alertmanager gửi; ack/silence hoạt động
- [ ] **Correlation demo**: panel error spike → exemplar → trace waterfall → logs của đúng request (1 click mỗi bước)
- [ ] Refresh token reuse → cả family bị revoke (test tự động)
- [ ] E2E pipeline smoke chạy nightly xanh

---

## Giai Đoạn 3 — SRE Workflow + Multi-Tenancy

**Mục tiêu:** Vòng SRE khép kín (SLO → burn-rate alert → incident → postmortem) + nhiều team dùng chung.

| # | Hạng mục |
|---|----------|
| 3.1 | `slo/` BC: define SLO → generate recording + MWMB burn-rate rules (bảng 14.4/6/1) → budget snapshots → compliance API |
| 3.2 | `notification/` BC: channels (Slack/Email/PagerDuty/Teams/webhook) + templates + Redis delivery queue + history; secrets mã hóa AES-GCM |
| 3.3 | `incident/` BC: lifecycle state machine + timeline + auto-create từ AlertFired/BudgetExhausted + MTTR/MTTA metrics |
| 3.4 | On-call schedule + escalation + override |
| 3.5 | Postmortem + action items |
| 3.6 | Multi-tenancy: workspaces + members + RBAC middleware + **per-workspace data streams** + per-workspace rate limit + audit logs đầy đủ |
| 3.7 | `logpipeline/` BC phần quản lý: pipeline status, DLQ view/retry, ILM editor |
| 3.8 | Frontend: SLO dashboard, incident board + timeline, on-call view, channel settings, workspace switcher |

**DoD GĐ3:**
- [ ] Define SLO 99.9%/28d → rules sinh tự động đúng công thức Workbook (test so sánh PromQL kỳ vọng); đốt budget bằng demo-order → page (fast burn) và ticket (slow burn) đúng kênh
- [ ] Critical alert > 5m → incident tự tạo + on-call nhận PagerDuty; resolve → incident đóng + MTTR ghi nhận
- [ ] User workspace A không thấy bất kỳ data nào của workspace B (test isolation tự động: API + log search + alerts)
- [ ] Notification delivery có history; kênh chết không làm mất notification kênh khác (circuit breaker test)

---

## Giai Đoạn 4 — Scale & Enterprise

**Mục tiêu:** Chịu tải production lớn, retention dài, vận hành chuyên nghiệp.

| # | Hạng mục |
|---|----------|
| 4.1 | Mode B: Kafka 4.3 (3 brokers KRaft) buffer + DLQ topic; ES 3 nodes; Alertmanager ×2 gossip |
| 4.2 | Thanos (sidecar/store/query/compactor) + S3 — SLO window dài + metrics 1 năm |
| 4.3 | Scheduled reports (SLO weekly, incident summary) + async export (S3, signed URL) |
| 4.4 | Service topology từ traces (materialized 30s, Redis cache) + health map UI |
| 4.5 | Usage/cost dashboard per workspace + ingestion quotas enforcement |
| 4.6 | (Tùy chọn) Kubernetes migration theo 10 mục 7 |

**DoD GĐ4:** Load test 10K logs/s duy trì 1h không mất log (đếm end-to-end); kill gateway 5 phút → Kafka buffer replay đủ; SLO 28d chạy trên Thanos; restore drill pass.

---

## Nguyên Tắc Thực Thi

1. **Tuần tự theo giai đoạn, song song trong giai đoạn** — các story trong cùng GĐ theo workflow agile (`doc/logmon-agile-agents.md`): BA story → tech-spec + openapi → QC test cases song song → implement → review.
2. **Không nhảy giai đoạn** — ví dụ không làm SLO trước khi có rule sync (GĐ2) vì SLO phụ thuộc pipeline đó.
3. **Mỗi GĐ kết thúc bằng demo DoD** — chạy đủ checklist trước khi mở GĐ kế.
4. **Story mapping**: mỗi hạng mục bảng trên ≈ 1-3 stories trong `stories/{bc}/...`.
