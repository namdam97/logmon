# 11 — Coding & Testing Standards

> Quy tắc Go đầy đủ đã nằm trong `CLAUDE.md` (nguồn: Uber Go Style Guide, Dave Cheney SOLID Go, OWASP Go-SCP) — file này KHÔNG lặp lại, chỉ ghi **delta cập nhật 2026** và **chiến lược testing** chi tiết.

---

## 1. Delta So Với Quy Tắc Hiện Hành (CLAUDE.md)

| Mục | Hiện tại | Cập nhật v2 | Lý do |
|-----|----------|-------------|-------|
| Go version | 1.22+ | **1.26.x** | Green Tea GC mặc định, container-aware GOMAXPROCS |
| Password | bcrypt | **argon2id** (m=19456, t=2, p=1) | OWASP hiện hành — cập nhật CLAUDE.md tương ứng (ADR-022) |
| Circuit breaker | sony/gobreaker | **sony/gobreaker/v2** | Generics API |
| Lint | golangci-lint | **golangci-lint v2** (`golangci-lint migrate` để chuyển config) | v2 đổi format config, thêm `fmt` |
| Logging | zerolog wrapper | Giữ zerolog (nhanh nhất) + **hook inject trace_id/span_id** từ `trace.SpanContextFromContext(ctx)` | OTel Go Logs còn Beta; slog-bridge của zerolog chậm |
| JWT | (chưa quy định lib) | **golang-jwt/jwt/v5**, pin algorithm, validate iss/aud | RFC 8725 |
| Rate limit | (tự viết sliding window) | **go-redis/redis_rate/v10** (GCRA) | Atomic Lua, chính xác hơn, không tự viết |
| Migrations | (chưa quy định) | **golang-migrate** (`migrate/migrate` container), định dạng `NNNNNN_name.up/down.sql`, zero-downtime rules (08) | |
| OTel instrumentation | (chưa có) | `otelgin`, `exaring/otelpgx`, `redisotel/v9` | Official/registry-listed |
| CI security | (chưa có) | `govulncheck` + `gitleaks` bắt buộc trong CI | |

Các quy tắc khác trong CLAUDE.md (error wrapping không "failed to", handle-once, ISP interfaces nhỏ, goroutine lifecycle stop/done, channel size 0/1, enum iota+1, table-driven tests give/want, `require` cho setup...) — **giữ nguyên, vẫn đúng chuẩn 2026**.

---

## 2. Chiến Lược Testing (3 tầng — bắt buộc đủ cả 3)

### 2.1 Unit Tests (nhanh, không I/O)

- **Đối tượng**: domain logic (aggregates, value objects, state machines), app handlers với mock ports, pure functions (burn-rate math, on-call resolution, template render).
- Table-driven, naming `tests/tt/give/want`; mock qua interfaces trong `ports/` (tự viết mock struct — không bắt buộc mockgen).
- Thời gian inject qua field `now func() time.Time` — KHÔNG mutable globals.
- **Ví dụ giá trị cao**: incident state machine (mọi transition hợp lệ/không hợp lệ), SLO budget math (burn rate 14.4 đúng công thức), webhook idempotency (cùng fingerprint+startsAt không tạo instance trùng).

### 2.2 Integration Tests (I/O thật, container hóa)

- **Công cụ**: `testcontainers-go` — spin Postgres/Redis/ES thật trong test; build tag `//go:build integration`, chạy `make test-integration` + CI job riêng.
- **Đối tượng**:
  - Repository adapters vs Postgres thật (CRUD + workspace filter + migrations chạy được từ 0).
  - Outbox relay: ghi event trong TX → relay nhặt → subscriber nhận → status published; kill giữa chừng → không mất event (at-least-once).
  - Rule syncer: render → `promtool check rules` pass → file ghi atomic.
  - ES adapter: index template áp dụng, search query có workspace filter.
- Mỗi test tự tạo schema/data riêng (isolation) — không share state giữa tests.

### 2.3 E2E Tests (user flows)

- **Frontend**: Playwright (đã có `frontend/e2e/auth.spec.ts`) — flows: login/logout, refresh hết hạn, tạo alert rule, xem active alerts, log search.
- **Pipeline E2E** (smoke, chạy trên compose trong CI nightly hoặc staging):
  1. demo-order sinh log/metric/trace → assert log xuất hiện trong ES (≤30s), metric scrape được, trace có trong Jaeger.
  2. Bắn error burst → alert rule fire → Alertmanager → LogMon webhook nhận → instance xuất hiện trong API.
  3. Correlation: lấy trace_id từ log → `GET /logs/trace/:id` trả đúng logs.

### 2.4 Coverage & Gates

| Gate | Ngưỡng |
|------|--------|
| Unit coverage (domain + app) | **≥ 80%** (`go test -cover`, enforce trong CI) |
| Adapters | Coverage qua integration tests (không ép 80% unit) |
| CI bắt buộc xanh | unit + lint + govulncheck + gitleaks (mọi PR); integration (PR vào main); e2e smoke (nightly/staging) |
| `go test -race` | Luôn bật |

### 2.5 TDD Workflow (giữ quy trình hiện hành)

RED (viết test fail trước) → GREEN (implement tối thiểu) → REFACTOR → verify coverage. Với bug fix: regression test tái hiện bug TRƯỚC khi sửa.

---

## 3. Frontend Standards (Next.js 16)

- TypeScript strict, không `any`; API client sinh từ `openapi.yaml` per story (workflow agile) — không viết tay types trùng backend.
- App Router + React 19; React Compiler bật (bớt useMemo/useCallback thủ công); `output: 'standalone'` cho Docker.
- State: server state qua fetch + cache tags; client state tối thiểu.
- Components: shadcn/ui; dashboard data-table theo hướng dẫn `ecc:dashboard-builder` + `ecc:design-system` (không dùng taste-skill cho dashboard).
- Testing: component tests (Vitest + Testing Library) cho logic UI; Playwright cho flows; a11y: form có label, error announce, keyboard nav (review bằng `ecc:a11y-architect`).

---

## 4. Definition of Done (mọi story)

- [ ] Tests đủ 3 tầng tương ứng phạm vi story; coverage domain/app ≥ 80%
- [ ] `golangci-lint v2` + `go vet` + `govulncheck` sạch
- [ ] Error handling đúng quy tắc (wrap ngắn gọn, handle-once, domain errors)
- [ ] Logging/metrics/tracing được thêm cho code path mới (đây là platform observability — dogfood chính mình)
- [ ] Security checklist (09 mục 9) cho code chạm auth/input/query
- [ ] Tài liệu cập nhật nếu lệch doc_v2 (doc là source of truth — lệch thì sửa doc qua PR cùng lúc)
