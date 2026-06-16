# 07 — REST API Specification

> Prefix `/api/v1/`. Mỗi endpoint đánh dấu **giai đoạn** triển khai. OpenAPI spec chi tiết per-story sinh trong `stories/{bc}/{feature}/tech/openapi.yaml` (theo workflow agile) — file này là catalog + conventions tổng.

---

## 1. Conventions

### 1.1 Response Envelope (thống nhất MỌI endpoint)

```json
// Thành công
{ "data": { ... }, "error": null, "meta": { "total": 42, "page": 1, "per_page": 20 } }

// Lỗi
{ "data": null, "error": { "code": "VALIDATION_ERROR", "message": "validation failed" }, "meta": null }
```

- `meta` chỉ xuất hiện ở list endpoints (pagination).
- Error message **generic** cho client; chi tiết log internally kèm trace_id. Client nhận `trace_id` qua header `X-Trace-Id` để báo lỗi.

| HTTP | code | Khi nào |
|------|------|---------|
| 400 | `VALIDATION_ERROR` | Input không hợp lệ |
| 401 | `AUTH_REQUIRED` | Thiếu/sai/hết hạn token |
| 403 | `FORBIDDEN` | Thiếu quyền (RBAC) hoặc CSRF fail |
| 404 | `NOT_FOUND` | Resource không tồn tại (hoặc khác workspace — không phân biệt để tránh lộ thông tin) |
| 409 | `CONFLICT` | Trùng tên/version conflict |
| 429 | `RATE_LIMITED` | Quá quota; kèm header `Retry-After` |
| 500 | `INTERNAL_ERROR` | Lỗi không xác định |

### 1.2 Pagination, Sorting, Filtering

```
GET /api/v1/alerts/rules?page=1&per_page=20&sort=created_at&order=desc&service=logmon-api
```
- `per_page` max 100. List response luôn có `meta.total`.
- Time range filter: `from`/`to` ISO8601 UTC.

### 1.3 Auth & Headers

- Access token: cookie `lm_access` (HttpOnly, Secure, SameSite=Strict, 15 phút).
- Refresh: `POST /auth/refresh` dùng cookie `lm_refresh` (path-scoped `/api/v1/auth`).
- CSRF: endpoints state-changing (POST/PUT/DELETE) yêu cầu header `X-CSRF-Token` (double-submit — chi tiết 09-security).
- Multi-tenancy (GĐ3): header `X-Workspace-ID` — middleware validate user là member.
- Idempotency (tùy chọn, cho POST quan trọng): header `Idempotency-Key`.

### 1.4 Versioning & Deprecation

- Path version `/v1`. Breaking change → `/v2` song song ≥ 6 tháng. Field mới = non-breaking (client phải bỏ qua field lạ).

---

## 2. Endpoint Catalog

### 2.1 Health & System — GĐ 1 ✅ (một phần đã có)

| Method | Endpoint | Mô tả | Auth |
|--------|----------|-------|------|
| GET | `/health` | Liveness (process sống) | No |
| GET | `/ready` | Readiness (check DB/ES/Redis dependencies) | No |
| GET | `/api/v1/system/info` | Version, uptime, component status | Admin |

### 2.2 Auth (identity BC) — GĐ 1 ✅ (login/logout đã có; refresh GĐ 2)

| Method | Endpoint | Mô tả | Auth |
|--------|----------|-------|------|
| POST | `/api/v1/auth/register` | Đăng ký (có thể tắt qua config — production thường admin-invite) | No |
| POST | `/api/v1/auth/login` | Login → set access + refresh cookies + CSRF token | No (rate-limited) |
| POST | `/api/v1/auth/logout` | Revoke refresh family + clear cookies | Yes |
| POST | `/api/v1/auth/refresh` | Rotate refresh token, cấp access mới | Refresh cookie |
| GET | `/api/v1/auth/me` | User hiện tại | Yes |

### 2.3 Alert Rules & Instances (alerting BC) — GĐ 2

| Method | Endpoint | Mô tả | Role |
|--------|----------|-------|------|
| GET | `/api/v1/alerts/rules` | List (filter: service, severity, enabled) | viewer |
| POST | `/api/v1/alerts/rules` | Tạo rule — validate PromQL + labels bắt buộc, trigger sync | editor |
| GET/PUT/DELETE | `/api/v1/alerts/rules/:id` | Chi tiết / sửa / xóa (sync lại) | viewer / editor / editor |
| GET | `/api/v1/alerts/rules/:id/state` | Trạng thái evaluation từ Prometheus `/api/v1/rules` | viewer |
| GET | `/api/v1/alerts/active` | Instances đang firing (read model) | viewer |
| POST | `/api/v1/alerts/:id/acknowledge` | Ack instance | editor |
| POST | `/api/v1/alerts/:id/silence` | Silence (duration + reason) → Alertmanager API v2 | editor |
| GET | `/api/v1/alerts/history` | Lịch sử (filter: service, severity, time range) | viewer |
| POST | `/api/v1/alerts/webhook` | **Internal** — Alertmanager webhook receiver | Internal bearer token |

Ví dụ tạo rule:

```json
POST /api/v1/alerts/rules
{
  "name": "high-error-rate",
  "expression": "rate(logmon_http_requests_total{status=~\"5..\"}[5m]) / rate(logmon_http_requests_total[5m]) > 0.05",
  "for": "2m",
  "severity": "critical",
  "service": "demo-order",
  "labels": { "team": "backend" },
  "annotations": {
    "summary": "Error rate > 5% on {{ $labels.service }}",
    "runbook_url": "https://wiki.internal/runbooks/high-error-rate"
  }
}
→ 201 { "data": { "id": "...", "sync_status": "synced", "created_at": "..." } }
→ 400 nếu PromQL sai cú pháp hoặc thiếu runbook_url
```

### 2.4 Log Search (logpipeline BC) — GĐ 2

| Method | Endpoint | Mô tả | Role |
|--------|----------|-------|------|
| POST | `/api/v1/logs/search` | Full-text + filters; max size 1000, timeout 10s | viewer |
| GET | `/api/v1/logs/tail?service=&level=` | SSE stream realtime; max 5 connections/workspace | viewer |
| GET | `/api/v1/logs/trace/:trace_id` | Logs theo trace_id | viewer |
| GET | `/api/v1/logs/stats` | Volume per service/level (cache 30s) | viewer |

### 2.5 Pipeline Management (logpipeline BC) — GĐ 2-3

| Method | Endpoint | Mô tả | Role |
|--------|----------|-------|------|
| GET | `/api/v1/pipeline/status` | Mode, throughput, collector/ES/Kafka health | viewer |
| POST | `/api/v1/pipeline/mode` | Switch A ↔ B (orchestrated, có confirm) | admin |
| GET | `/api/v1/pipeline/dlq` | DLQ count + samples | viewer |
| POST | `/api/v1/pipeline/dlq/retry` | Retry sau review (chọn entries) | admin |
| GET/PUT | `/api/v1/pipeline/ilm` | Xem / sửa ILM policy (gọi ES API) | viewer / admin |
| GET | `/api/v1/pipeline/datastreams` | Data stream stats (size, doc count, backing indices) | viewer |

### 2.6 SLO (slo BC) — GĐ 3

| Method | Endpoint | Mô tả | Role |
|--------|----------|-------|------|
| GET / POST | `/api/v1/slos` | List / define (trigger rule generation) | viewer / editor |
| GET/PUT/DELETE | `/api/v1/slos/:id` | Chi tiết + compliance / sửa / xóa | viewer / editor / editor |
| GET | `/api/v1/slos/:id/budget` | Budget remaining + burn rates (1h/6h/24h) | viewer |
| GET | `/api/v1/slos/compliance` | Tổng quan tất cả SLO | viewer |

```json
GET /api/v1/slos/slo-001/budget → 200
{ "data": {
  "target": 0.999, "window": "28d", "current_sli": 0.9995,
  "budget_total_minutes": 40.3, "budget_remaining_minutes": 26.9, "budget_remaining_percent": 66.7,
  "burn_rate_1h": 2.1, "burn_rate_6h": 1.4, "burn_rate_24h": 0.8,
  "status": "healthy"
}}
```

### 2.7 Incidents & On-call (incident BC) — GĐ 3

| Method | Endpoint | Mô tả | Role |
|--------|----------|-------|------|
| GET / POST | `/api/v1/incidents` | List (filter status/severity/service) / tạo manual | viewer / editor |
| GET | `/api/v1/incidents/:id` | Chi tiết + timeline | viewer |
| PUT | `/api/v1/incidents/:id/triage` | Set severity + impact | editor |
| PUT | `/api/v1/incidents/:id/assign` | Assign | editor |
| PUT | `/api/v1/incidents/:id/status` | mitigating / resolved | editor |
| POST | `/api/v1/incidents/:id/timeline` | Thêm timeline entry | editor |
| POST / GET | `/api/v1/incidents/:id/postmortem` | Submit / xem postmortem | editor / viewer |
| GET | `/api/v1/incidents/metrics` | MTTR/MTTA aggregate | viewer |
| GET | `/api/v1/oncall/current` | Ai đang on-call | viewer |
| GET / PUT | `/api/v1/oncall/schedule` | Xem / sửa rotation | viewer / admin |
| POST | `/api/v1/oncall/override` | Override tạm thời | editor |

### 2.8 Notifications (notification BC) — GĐ 3

| Method | Endpoint | Mô tả | Role |
|--------|----------|-------|------|
| GET / POST | `/api/v1/notifications/channels` | List / thêm kênh | admin |
| PUT / DELETE | `/api/v1/notifications/channels/:id` | Sửa / xóa | admin |
| POST | `/api/v1/notifications/channels/:id/test` | Gửi test | admin |
| GET / PUT | `/api/v1/notifications/templates` | List / sửa template | admin |
| GET | `/api/v1/notifications/history` | Delivery history | viewer |

### 2.9 Workspaces & RBAC (identity BC) — GĐ 3

| Method | Endpoint | Mô tả | Role |
|--------|----------|-------|------|
| GET / POST | `/api/v1/workspaces` | List của user / tạo | yes / platform_admin |
| GET / POST | `/api/v1/workspaces/:id/members` | List / invite | viewer / admin |
| PUT / DELETE | `/api/v1/workspaces/:id/members/:uid` | Đổi role / xóa | admin |

Roles: `viewer` (read) ⊂ `editor` (CRUD rules/SLO/incidents) ⊂ `admin` (members, channels, pipeline, mode switch) ⊂ `platform_admin` (mọi workspace).

### 2.10 Reports, Export, Topology, Usage — GĐ 4

| Method | Endpoint | Mô tả |
|--------|----------|-------|
| GET/POST/PUT/DELETE | `/api/v1/reports/schedules*` | Quản lý scheduled reports (SLO weekly, incident summary, cost monthly) |
| POST | `/api/v1/reports/generate` | On-demand |
| POST | `/api/v1/export/logs` \| `/export/metrics` | Async export (202 + job id, poll `/export/jobs/:id`, file lên S3, URL hết hạn 24h) |
| GET | `/api/v1/topology` | Service dependency graph từ traces (materialized 30s, cache Redis TTL 5m) |
| GET | `/api/v1/usage` | Per-workspace ingestion/storage usage + ước tính chi phí |

---

## 3. Rate Limits (mặc định, per workspace — GĐ1 per IP)

| Nhóm | Limit |
|------|-------|
| Auth (login/refresh) | 10 req/min/IP (chống brute force) |
| Log search | 100 req/min |
| Log tail SSE | 5 concurrent connections |
| Mutations (rules, SLO, incidents) | 30 req/min |
| Còn lại | 1000 req/min |

Implementation: `redis_rate/v10` (GCRA). Trả 429 + `Retry-After`. Fail-open khi Redis down (log + metric).
