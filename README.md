# LogMon

> Nền tảng **observability** (logging + monitoring) self-hosted cho hệ Go microservices — thu thập **logs, metrics, traces**, cảnh báo theo SLO, và một **admin dashboard** để vận hành.

LogMon vừa là **sản phẩm** (platform quan sát hệ thống) vừa là **bộ khung tham chiếu** cho cách xây microservice Go đúng chuẩn: Clean Architecture, DDD/CQRS cho domain phức tạp, pipeline log/metrics chuẩn OpenTelemetry, và quy trình dev/test local nhất quán.

---

## Vấn đề nó giải quyết

Khi chạy nhiều microservice, đội ngũ thường vấp phải:

- **Log rải rác** mỗi service một kiểu, khó truy vết một request xuyên nhiều service.
- **Không biết hệ thống "khoẻ" hay không** — thiếu metrics chuẩn, thiếu cảnh báo sớm.
- **Cảnh báo nhiễu** (alert fatigue) hoặc bỏ sót sự cố vì không có SLO/error budget.
- **Vào việc chậm**: mỗi tính năng đụng cả FE + BE + hạ tầng, dựng môi trường local lích kích, mỗi máy một kiểu.

LogMon gom các mảnh đó thành một stack mạch lạc: **OpenTelemetry → Elasticsearch (logs)**, **Prometheus (metrics) → Alertmanager (cảnh báo theo severity/SLO)**, **Grafana (trực quan hoá)**, cộng một **admin dashboard** và một **vòng lặp dev/test một lệnh**.

---

## Tech Stack

| Mảng | Công nghệ |
|------|-----------|
| **Backend** | Go 1.25 · Gin · pgx/v5 (Postgres) · zerolog · prometheus/client_golang · JWT (cookie HttpOnly) · golang-migrate |
| **Frontend** | Next.js 14 (App Router, TypeScript) · pnpm · TailwindCSS · shadcn/ui |
| **Metrics** | Prometheus (pull) → Alertmanager → Slack/Email · node/postgres/es exporters |
| **Logs** | OpenTelemetry Collector (agent → gateway) → Elasticsearch (data streams, ILM) |
| **Trực quan hoá** | Grafana (dashboards metrics + logs) · admin dashboard (Next.js) |
| **Hạ tầng** | Docker Compose (profiles: nhẹ ↔ full) · CI GitHub Actions |
| **Test** | `go test -race` · Vitest · Playwright (E2E full-stack) |

> Bảng trên là **hiện trạng repo** (walking skeleton). **Target 06/2026** (xem [`doc_v2/01-kien-truc-tong-the.md`](doc_v2/01-kien-truc-tong-the.md)): Go 1.26 · **Next.js 16.2** · `identity` thay `user` · **argon2id** thay bcrypt · Grafana 13.1.

---

## Kiến trúc (tóm tắt)

Backend theo **2 mô hình** tuỳ độ phức tạp domain (chi tiết: [`doc_v2/02-backend-architecture.md`](doc_v2/02-backend-architecture.md)):

- **Clean Architecture** — `user` (→ `identity`), `notification` (CRUD, domain đơn giản).
- **Clean Arch + DDD + CQRS** — `alerting`, `slo`, `logpipeline`, `incident` (business rule phức tạp).

Hướng phụ thuộc một chiều: `adapters → ports ← app → domain` (domain chỉ import stdlib). Các bounded context giao tiếp qua **domain events**, không import chéo.

Pipeline log (Mode A): mỗi container ghi JSON log → **OTel agent** đọc → **OTel gateway** → **Elasticsearch**. Metrics: **Prometheus** scrape service + exporters → **Alertmanager** định tuyến theo severity → Slack/deadman. Xem [`doc_v2/03-logs-pipeline.md`](doc_v2/03-logs-pipeline.md), [`doc_v2/04-metrics-tracing.md`](doc_v2/04-metrics-tracing.md), [`doc_v2/05-alerting-slo.md`](doc_v2/05-alerting-slo.md).

### Trạng thái hiện tại

| Đã có | Đang/ sẽ làm |
|-------|--------------|
| `user` BC: đăng ký / đăng nhập / đăng xuất / `/me` (JWT cookie, bcrypt, rate limit) | `identity` (đổi tên từ `user`), `alerting`, `slo`, `logpipeline`, `incident`, `notification` BC |
| Shared kernel: logger, errors, httpx envelope, metrics, middleware, auth | Domain events cross-BC |
| Observability stack: OTel→ES, Prometheus, Grafana (4 dashboards), Alertmanager | Mode B (Kafka buffer, ES cluster) |
| Admin dashboard: login / tổng quan / hồ sơ | Bảng users, role admin, widget metrics |
| `demo-order` (workload sinh telemetry) + loadgen | — |
| CI (test-race, lint, govulncheck, gitleaks) + E2E Playwright | — |

---

## Bắt đầu nhanh (Quick Start)

### Yêu cầu

- **Docker** + **Docker Compose** (profiles)
- **Go 1.25+**
- **Node + pnpm**
- **Google Chrome** (cho Playwright E2E — dùng Chrome hệ thống)
- **make** · *(tuỳ chọn)* `golangci-lint` cho `make lint`

Kiểm tra nhanh toolchain:

```bash
make doctor
```

### Chạy stack + dashboard (vọc trong ~2 phút)

```bash
# 1. (tuỳ chọn) cấu hình env
cp .env.example .env

# 2. Dựng stack NHẸ: postgres + migrate + userservice (API :8080)
make up

# 3. Nạp dữ liệu demo (idempotent)
make seed
#   → admin@logmon.local / password123
#   → dev@logmon.local   / password123

# 4. Chạy frontend (terminal khác)
cd frontend && pnpm install && pnpm dev   # hoặc: make dev-fe

# 5. Mở http://localhost:3000/login và đăng nhập bằng tài khoản demo ở trên
```

Muốn cả **observability** (Grafana/Prometheus/ES/OTel):

```bash
make up-full       # thêm profile observability (nặng hơn — ES ~1GB RAM)
make up-demo       # full + demo workload sinh telemetry liên tục
```

### Phát triển có hot reload

```bash
make dev           # dựng DB + migrate, in hướng dẫn
make dev-be        # Go API hot reload (air) — terminal riêng
make dev-fe        # Next.js dev server  — terminal riêng
```

### Lệnh hằng ngày

| Lệnh | Tác dụng |
|------|----------|
| `make up` / `up-full` / `up-demo` | Dựng stack (nhẹ / + observability / + demo) |
| `make down` / `down-v` | Dừng (giữ data / xoá sạch volume) |
| `make migrate` / `migrate-down` | Áp / rollback migration (golang-migrate) |
| `make seed` | Nạp user demo (idempotent) |
| `make test` | Unit test BE (`go -race`) + FE (vitest) |
| `make e2e` | E2E full-stack: tự dựng BE+DB, chạy Playwright, teardown |
| `make ci-local` | Mô phỏng CI (lint + test + e2e) |
| `make fmt` / `lint` | Format / lint backend |
| `make logs` / `ps` | Xem log / liệt kê container |

`make help` để xem đầy đủ.

---

## Cổng dịch vụ

| Dịch vụ | URL | Profile |
|---------|-----|---------|
| Frontend (admin) | http://localhost:3000 | (host) |
| userservice API | http://localhost:8080 (`/healthz`, `/metrics`) | mặc định |
| Postgres | `localhost:5432` (logmon/logmon) | mặc định |
| Grafana | http://localhost:3001 (admin / `GRAFANA_ADMIN_PASSWORD`) | observability |
| Prometheus | http://localhost:9090 | observability |
| Alertmanager | http://localhost:9093 | observability |
| Elasticsearch | http://localhost:9200 | observability |
| demo-order | http://localhost:8081 | demo |

---

## Cấu trúc thư mục

```
backend/            # Go services
  cmd/              #   userservice (API), seeder (dữ liệu demo)
  internal/
    user/           #   bounded context: domain / app / ports / adapters
    shared/         #   shared kernel: logger, errors, httpx, metrics, middleware, auth
  migrations/       #   golang-migrate (000001_init.{up,down}.sql)
frontend/           # Next.js admin dashboard (App Router) + Playwright e2e/
infra/
  docker/           # docker-compose.yml (profiles: observability, demo) + secrets/
  prometheus/ alertmanager/ grafana/ otel/ elasticsearch/   # config observability
examples/demo-order/# service demo sinh telemetry để thử platform
doc_v2/             # tài liệu thiết kế (source of truth) — 00..09 + ADRs
Makefile            # điều phối toàn bộ vòng lặp local
CLAUDE.md           # hướng dẫn cho AI agent + quy ước code/kiến trúc
```

---

## Test

```bash
cd backend && go test -race -cover ./...    # unit + race (chuẩn CI)
make test                                   # BE + FE unit
make e2e                                    # full-stack Playwright (5 luồng auth)
```

CI (`.github/workflows/ci.yml`) gồm: build + `go test -race` + golangci-lint + govulncheck (backend), vitest + `next build` (frontend), gitleaks (secrets), validate Prometheus/compose config, build images.

---

## Tài liệu

- **[`doc_v2/`](doc_v2/)** — thiết kế chi tiết (kiến trúc, logs pipeline, metrics/tracing, alerting/SLO, API spec, DB schema, security). Là **source of truth**.
- **[`CLAUDE.md`](CLAUDE.md)** — quy ước code (Go style, layer rules, security) + hướng dẫn cho AI agent.

> Tài liệu dự án viết bằng tiếng Việt.
