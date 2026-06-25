# LogMon — Tài Liệu Thiết Kế v2 (Production-Ready)

> **Phiên bản:** 2.0 — 2026-06-11
> **Thay thế:** `doc/logmon.md` v1 (2026-04-02, đã gỡ khỏi repo — xem git history) và các tài liệu liên quan
> **Cơ sở:** Toàn bộ thay đổi so với v1 đều dựa trên research best practices production tính đến 06/2026, có nguồn kiểm chứng (xem `13-adr.md`).

---

## Mục Lục Bộ Tài Liệu

| # | File | Nội dung | Đọc khi nào |
|---|------|----------|-------------|
| 00 | [00-tong-quan.md](00-tong-quan.md) | Vision, bài toán, personas, phạm vi & non-goals | Bắt đầu |
| 01 | [01-kien-truc-tong-the.md](01-kien-truc-tong-the.md) | Kiến trúc hệ thống, tech stack (pinned versions), 4 luồng dữ liệu, 2 deployment modes | Bắt đầu |
| 02 | [02-backend-architecture.md](02-backend-architecture.md) | Bounded Contexts, Clean Arch + DDD + CQRS, layer rules, domain events, outbox | Trước khi code backend |
| 03 | [03-logs-pipeline.md](03-logs-pipeline.md) | OTel Collector pipeline, Kafka KRaft, ES data streams + ILM, DLQ, multi-tenant | Triển khai logs |
| 04 | [04-metrics-tracing.md](04-metrics-tracing.md) | Prometheus 3.x, Thanos, OTel SDK/Collector, Jaeger v2, correlation | Triển khai metrics/traces |
| 05 | [05-alerting-slo.md](05-alerting-slo.md) | Alerting BC, rule sync, Alertmanager, SLO engine, burn-rate, meta-monitoring | Triển khai alerting |
| 06 | [06-incident-notification.md](06-incident-notification.md) | Incident lifecycle, on-call, Notification Hub | Giai đoạn 3 |
| 07 | [07-api-specification.md](07-api-specification.md) | API conventions, response envelope, endpoint catalog (đánh dấu theo giai đoạn) | Trước khi code API |
| 08 | [08-database-schema.md](08-database-schema.md) | PostgreSQL schema, migrations (golang-migrate), ERD | Trước khi code backend |
| 09 | [09-security.md](09-security.md) | AuthN/Z, argon2id, JWT + refresh rotation, CSRF, tenancy isolation, OWASP | Trước khi code auth |
| 10 | [10-deployment-operations.md](10-deployment-operations.md) | Docker Compose production, CI/CD, backup/DR, capacity, đường lên K8s | Triển khai hạ tầng |
| 11 | [11-coding-testing-standards.md](11-coding-testing-standards.md) | Go style (delta so CLAUDE.md), testing strategy, coverage | Trước khi code |
| 12 | [12-roadmap.md](12-roadmap.md) | Lộ trình 5 giai đoạn, map với trạng thái repo hiện tại, Definition of Done | Lập kế hoạch |
| 13 | [13-adr.md](13-adr.md) | Toàn bộ ADR: giữ nguyên / cập nhật / mới (kèm nguồn research) | Hiểu quyết định |
| 14 | [14-frontend-architecture.md](14-frontend-architecture.md) | Kiến trúc FE (Next.js admin UI): phân tầng, data layer, auth/RBAC, màn hình theo giai đoạn, ranh giới với Grafana | Trước khi code frontend |
| 15 | [15-devsecops-cicd.md](15-devsecops-cicd.md) | CI/CD pipeline (hiện trạng `ci.yml` + mục tiêu), security gates tự động, supply-chain, secret rotation, môi trường/deploy/rollback | Trước khi đụng CI/CD & release |
| 16 | [16-iac-runbooks.md](16-iac-runbooks.md) | Infrastructure-as-Code (cấu trúc `infra/`, compose, config-as-code, đường lên K8s) + runbook cho 8 alert nền & quy trình vận hành | Triển khai hạ tầng & vận hành |
| 17 | [17-ai-incident-automation.md](17-ai-incident-automation.md) | GĐ5: AI hỗ trợ chẩn đoán sự cố (RCA) + RAG runbook/postmortem, human-in-the-loop, giảm MTTR | Giai đoạn 5 |

**Thứ tự đọc đề xuất:** 00 → 01 → 12 (nắm lộ trình) → 02 → file chuyên đề theo giai đoạn đang làm (backend: 03-09; frontend: 14; CI/CD & hạ tầng: 15-16; AI xử lý sự cố/GĐ5: 17).

> **Quy ước trạng thái trong 14-16:** ✅ đã có trong repo · 📐 đã chốt trong doc_v2 (chưa triển khai) · ⬜ khoảng trống chưa quyết (cần ADR). Đây là tài liệu tham chiếu khi triển khai — không trình bày thứ chưa quyết như đã quyết.

---

## Thay Đổi Chính So Với v1

Đây là những thay đổi **bắt buộc** vì v1 dùng công nghệ đã lỗi thời hoặc lệch best practice production:

| # | v1 (04/2026) | v2 (06/2026) | Lý do (có nguồn trong ADR) |
|---|--------------|--------------|----------------------------|
| 1 | Filebeat → Kafka → Logstash → ES | **OTel Collector (filelog)** → [Kafka] → **OTel Collector gateway** → ES | Filebeat mất log trước 10K logs/s (benchmark 2026); Logstash JVM nặng 1-4GB; Elastic chính thức xoay trục sang OTel. 1 collector cho cả logs + traces + metrics → ADR-018 |
| 2 | Kafka + Zookeeper | **Kafka 4.3 KRaft-only** | ZooKeeper bị xóa hoàn toàn từ Kafka 4.0 (03/2025) → ADR-027 |
| 3 | Index `logs-{ws}-{svc}-{yyyy.MM.dd}` (daily) | **Data streams + ILM rollover** theo `max_primary_shard_size: 50gb` | Daily indices gây shard explosion; Elastic khuyến nghị chính thức rollover theo size → ADR-019 |
| 4 | Jaeger (v1, ES backend) | **Jaeger v2.19** (nền OTel Collector, ES backend) | Jaeger v1 EOL 31/12/2025 → ADR-020 |
| 5 | MinIO cho object storage | **Cloud S3/B2** (ưu tiên) hoặc SeaweedFS (on-prem) | MinIO gỡ tính năng quản trị khỏi community edition (2025) → ADR-021 |
| 6 | bcrypt cho password | **argon2id** (19 MiB, t=2, p=1) | Khuyến nghị OWASP hiện hành; bcrypt chỉ cho legacy → ADR-022 |
| 7 | JWT cookie 30 phút, không refresh | **Access 15m + refresh rotation + reuse detection + CSRF token** | OWASP: SameSite không đủ làm phòng tuyến duy nhất → ADR-023 |
| 8 | Burn rate threshold đơn (14.4) | **Multiwindow multi-burn-rate** đầy đủ (14.4x/6x/1x, cặp window 5m-1h/30m-6h/6h-3d) | Công thức chuẩn Google SRE Workbook Ch.5 → ADR-025 |
| 9 | Alerting BC tự quản lý rule (mơ hồ cách sync) | **Pipeline rõ ràng**: DB → render YAML → `promtool check` → atomic write → `POST /-/reload` | Prometheus không có (và sẽ không có) API ghi rule → ADR-024 |
| 10 | Không có meta-monitoring | **Watchdog deadman-switch** → healthchecks.io + external uptime check | "Ai canh người canh gác" — pattern chuẩn kube-prometheus → ADR-026 |
| 11 | `order/`, `user/` là BC demo lẫn trong platform | **Tách bạch**: `identity/` là BC platform thật; demo workload chuyển sang `examples/` | Platform và sample app không được lẫn nhau → ADR-029 |
| 12 | Scope dàn trải (8 BC, ~80 endpoints cùng lúc) | **Lộ trình 5 giai đoạn** với Definition of Done, map với repo hiện tại | Triển khai được thực tế — xem `12-roadmap.md` |
| 13 | Spanmetrics processor | **Spanmetrics connector** (+ exemplars) | Processor đã bị gỡ khỏi OTel Collector contrib |
| 14 | Versions: Go 1.22, Grafana 10.4, ES 8.x... | **Pinned 06/2026**: Go 1.26, ES 9.4.2, Kafka 4.3, Prometheus 3.12, Grafana 13.1.x, Next.js 16.2 | Bảng version đầy đủ trong `01-kien-truc-tong-the.md` |

**Những gì v1 đã đúng và v2 giữ nguyên:** Thanos sidecar pattern (research xác nhận vẫn là best practice), Prometheus PULL model, Clean Arch + DDD + CQRS cho BC phức tạp, transactional outbox, workspace multi-tenancy, Grafana single pane, tail sampling policies, ELK thay Loki (full-text search), 2 deployment modes.

---

## Phạm Vi Tài Liệu

- Bộ tài liệu này mô tả **LogMon platform** — không bao gồm quy trình làm việc agile/AI agents (vẫn ở `doc/logmon-agile-agents.md`) và thiết kế npx tool (`doc/npx-tool-design.md`).
- `doc/` v1 giữ lại làm tham khảo lịch sử; **mọi quyết định mới theo `doc_v2/`**.
