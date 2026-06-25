# 14 — Kiến Trúc Frontend

> Admin UI `logmon-frontend` (Next.js App Router). **Nguyên tắc cốt lõi:** UI mỏng, có chủ đích — Grafana gánh phần trực quan hóa dữ liệu time-series nặng (xem [04](04-metrics-tracing.md)); Next.js chỉ làm **control plane** (CRUD cấu hình, workflow SRE, quản trị) + những view dữ liệu mà Grafana không phục vụ tốt (log search tương tác, incident board, on-call). FE tiêu thụ REST `/api/v1` ([07](07-api-specification.md)) — không gọi thẳng ES/Prometheus/Jaeger.

---

## 1. Phạm Vi & Ranh Giới (FE làm gì / KHÔNG làm gì)

| FE (Next.js) ĐẢM NHẬN | Giao cho Grafana / nơi khác |
|------------------------|------------------------------|
| Alert rule CRUD, ack, silence | Time-series panel (latency/error/throughput) — Grafana |
| Active alerts, alert history (bảng) | Log volume timeline, RED metrics — Grafana |
| Log search tương tác + tail SSE + by-trace | Trace waterfall — deep-link sang Jaeger/Grafana Traces |
| Incident board, timeline, postmortem editor | Dashboard tổng hợp đa service — Grafana |
| On-call schedule + override | SLO burn-rate panel chi tiết — Grafana (FE chỉ hiển thị gauge tóm tắt) |
| SLO định nghĩa + bảng compliance | |
| Notification channels + templates + delivery history | |
| Workspace switcher + members + RBAC | |
| Pipeline status, DLQ review/retry, ILM editor | |

**Nguyên tắc ranh giới:**
- **Không reimplement Grafana.** Nơi nào cần khám phá metric tự do → nhúng/deep-link Grafana (mục 11), không vẽ lại chart engine.
- **Không gọi datastore trực tiếp.** Mọi truy vấn qua `/api/v1` (RBAC + workspace filter enforced ở backend). FE không giữ ES DSL / PromQL ngoài ô input do user nhập (validate phía backend).
- **Stateless về dữ liệu nghiệp vụ.** Không cache dữ liệu nhạy cảm trong localStorage; nguồn sự thật là server.

---

## 2. Tech Stack (mục tiêu vs hiện trạng)

| Hạng mục | Mục tiêu v2 (pinned) | Hiện trạng repo (2026-06) | Ghi chú |
|----------|----------------------|----------------------------|---------|
| Framework | **Next.js 16.2** App Router | Next.js 14.2.5 | Cần nâng cấp — xem mục 17 |
| React | **19.2** + React Compiler | React 18.3.1 | React Compiler bật khi lên 19 → bớt `useMemo/useCallback` thủ công |
| Ngôn ngữ | TypeScript strict, không `any` | TS 5.5 strict ✅ | `tsconfig` đã `strict: true` |
| Styling | TailwindCSS 3.4 + CSS variables | Tailwind 3.4 ✅ | dark mode `class`, base color `slate` |
| Component | shadcn/ui (style `new-york`, RSC) | shadcn/ui ✅ (button/card/input/label) | data-table sẽ thêm ở GĐ2 |
| Icon | lucide-react | ✅ | |
| Server-state | **TanStack Query v5** | ❌ chưa có (raw `fetch` + `useState`) | thêm ở GĐ2 (mục 4) |
| API types | sinh từ `openapi.yaml` (codegen) | ❌ viết tay trong `lib/api.ts` | thêm ở GĐ2 (mục 4) |
| Form/validation | react-hook-form + zod | ❌ chưa có | dùng khi có form CRUD GĐ2 |
| Test | Vitest + Testing Library + Playwright + axe | Vitest + Playwright ✅ (a11y chưa) | xem mục 14 |
| Build | `output: 'standalone'` cho Docker | ❌ chưa bật | thêm khi đóng gói (mục 16) |

> Quy ước style/test FE tổng quát: [11 mục 3](11-coding-testing-standards.md). File này **không lặp lại**, chỉ bổ sung kiến trúc chi tiết.

---

## 3. Phân Tầng & Cấu Trúc Thư Mục

FE theo mô hình **layer hướng feature**, song song với BC backend nhưng KHÔNG ánh xạ 1-1 (UI gộp nhiều BC theo workflow người dùng):

```
frontend/
├── app/                          ← App Router (routing + RSC shell)
│   ├── layout.tsx                ← root: html/body, providers (QueryClient, Theme, Toast)
│   ├── login/page.tsx            ← public route
│   └── (dashboard)/              ← route group: protected, dùng DashboardShell
│       ├── layout.tsx            ← AuthGuard + shell
│       ├── page.tsx              ← Tổng quan
│       ├── alerts/               ← GĐ2: rules, active, history
│       ├── logs/                 ← GĐ2: search, tail
│       ├── slo/                  ← GĐ3
│       ├── incidents/            ← GĐ3
│       ├── oncall/               ← GĐ3
│       ├── notifications/        ← GĐ3
│       ├── pipeline/             ← GĐ2-3
│       └── settings/             ← workspace, members
├── features/                     ← (GĐ2+) logic per domain: hooks + components + schema
│   └── alerts/
│       ├── api.ts                ← query/mutation hooks (TanStack Query) cho BC này
│       ├── schema.ts             ← zod schema form + type suy ra
│       └── components/           ← RuleForm, RuleTable, SeverityBadge...
├── components/
│   ├── ui/                       ← shadcn primitives (button, card, input, table...)
│   └── layout/                   ← DashboardShell, Sidebar, Topbar
├── lib/
│   ├── api/                      ← http client lõi (fetch wrapper, envelope, error)
│   │   ├── client.ts             ← request<T>(), credentials, CSRF, trace_id
│   │   └── generated/            ← types sinh từ openapi.yaml (không sửa tay)
│   ├── auth.ts                   ← session helpers, RBAC guard (can(role, action))
│   └── utils.ts                  ← cn(), formatters
└── e2e/                          ← Playwright specs
```

**Quy ước:**
- **RSC mặc định** — component là Server Component trừ khi cần interactivity (`"use client"` chỉ ở lá cây tương tác: form, table có sort, SSE viewer).
- **Feature folder** đóng gói cả 3 thứ: data hooks + zod schema + component. Trang trong `app/` chỉ compose feature, không chứa logic nghiệp vụ.
- **`lib/api/client.ts` là điểm duy nhất** chạm `fetch` — feature dùng hooks, không gọi `fetch` trực tiếp (DRY + dễ inject CSRF/trace_id/retry).

---

## 4. Tầng Dữ Liệu (Data Layer)

### 4.1 HTTP client lõi

Một wrapper duy nhất bao `fetch`, thống nhất với envelope [07 §1.1](07-api-specification.md):

```ts
// Envelope chuẩn theo 07: { data, error: {code,message} | null, meta? }
export interface Envelope<T> {
  data: T | null;
  error: { code: string; message: string } | null;
  meta?: { total: number; page: number; per_page: number };
}

export class ApiError extends Error {
  constructor(
    readonly code: string,        // VALIDATION_ERROR | AUTH_REQUIRED | FORBIDDEN | ...
    readonly status: number,
    message: string,
    readonly traceId?: string,    // từ header X-Trace-Id — hiển thị để user báo lỗi
  ) { super(message); }
}

async function request<T>(path: string, init?: RequestInit): Promise<{ data: T; meta?: Envelope<T>["meta"] }> {
  const res = await fetch(`${API_BASE_URL}${path}`, {
    ...init,
    credentials: "include",                       // gửi cookie HttpOnly qua origin
    headers: withCsrf(init),                      // chèn X-CSRF-Token cho mutation (mục 6)
  });
  const traceId = res.headers.get("X-Trace-Id") ?? undefined;
  const body = (await res.json()) as Envelope<T>;
  if (!res.ok || body.error) {
    throw new ApiError(body.error?.code ?? "INTERNAL_ERROR", res.status, body.error?.message ?? "request failed", traceId);
  }
  return { data: body.data as T, meta: body.meta };
}
```

### 4.2 Type-safety qua OpenAPI codegen

- Backend sinh `stories/{bc}/{feature}/tech/openapi.yaml` per story ([07](07-api-specification.md) header, [11 §3](11-coding-testing-standards.md)).
- FE chạy codegen (`openapi-typescript` hoặc `orval`) → `lib/api/generated/` — **không viết tay type trùng backend**. Đây là hợp đồng chống drift (mục 17).

### 4.3 Server-state qua TanStack Query

| Loại | Cách làm |
|------|----------|
| Read (list/detail) | `useQuery` với `queryKey` chứa filter + `workspaceId`; `staleTime` ngắn cho dữ liệu động (active alerts 10s) |
| Write (CRUD) | `useMutation` → `invalidateQueries` các key liên quan; optimistic update cho ack/silence |
| Polling | active alerts / pipeline status: `refetchInterval` (ví dụ 10s) thay vì WebSocket |
| Realtime | log tail / in-app notification: SSE (mục 10), không qua Query |

`QueryClient` đặt ở `app/layout.tsx` qua provider; `workspaceId` đưa vào mọi `queryKey` để cache không lẫn giữa workspace (GĐ3).

### 4.4 Error mapping → UI

`ApiError.code` map sang xử lý UI nhất quán:

| code | Xử lý FE |
|------|----------|
| `AUTH_REQUIRED` (401) | Thử `POST /auth/refresh` 1 lần; thất bại → redirect `/login` |
| `FORBIDDEN` (403) | Toast "không đủ quyền"; ẩn nút nếu RBAC biết trước (mục 6) |
| `VALIDATION_ERROR` (400) | Hiển thị lỗi tại field (map qua react-hook-form) |
| `RATE_LIMITED` (429) | Toast + tôn trọng `Retry-After`; disable nút tạm thời |
| `INTERNAL_ERROR` (500) | Toast generic + hiện `traceId` để user copy báo lỗi |

---

## 5. Quản Lý State (3 loại, tách bạch)

| Loại state | Công cụ | Ví dụ |
|------------|---------|-------|
| **Server state** | TanStack Query (nguồn sự thật ở backend) | rules, alerts, incidents, SLO |
| **URL state** | searchParams (App Router) | filter bảng (service/severity/time range), page, tab đang mở |
| **Client state** | `useState`/`useReducer` cục bộ; Zustand chỉ khi thật cần global | trạng thái mở/đóng dialog, form nháp |

**Nguyên tắc:** filter + phân trang phải ở **URL** (chia sẻ link được, back/forward đúng), không nhét vào React state. Không dùng global store cho dữ liệu server (Query đã cache).

---

## 6. Auth, Session & RBAC trên FE

### 6.1 Session (khớp [09](09-security.md) + [07 §1.3](07-api-specification.md))

- Token nằm trong **cookie HttpOnly** (`lm_access` 15m, `lm_refresh` path-scoped) — JS **không đọc được** (chống XSS chiếm token). FE chỉ biết "đã đăng nhập" qua `GET /api/v1/auth/me`.
- **CSRF double-submit:** login trả CSRF token (cookie đọc được + body); FE gửi lại qua header `X-CSRF-Token` cho mọi mutation. `withCsrf()` trong client lõi tự chèn (mục 4.1).
- **Refresh flow:** khi `request` gặp 401, gọi `POST /auth/refresh` đúng **một lần** (single-flight để tránh refresh đồng thời), retry request gốc; thất bại → `/login`. Reuse detection ở backend ([09](09-security.md), ADR-023).

### 6.2 Bảo vệ route

- **Hiện trạng:** `AuthGuard` (client component) gọi `me()`, chưa đăng nhập → `router.replace('/login')`. Đủ cho GĐ1.
- **Mục tiêu GĐ2+:** thêm **Next.js middleware** kiểm tra sự hiện diện cookie ở edge → redirect *trước khi* render (tránh flash nội dung protected). `AuthGuard` vẫn giữ để lấy `user`/role cho UI. Hai lớp bổ trợ, không thay thế nhau.

### 6.3 RBAC-aware UI (GĐ3)

- Role: `viewer ⊂ editor ⊂ admin ⊂ platform_admin` ([07 §2.9](07-api-specification.md)).
- Helper `can(role, action)` ẩn/disable nút theo role (UX) — **nhưng không phải biên bảo mật**; backend luôn enforce (defense in depth). Ví dụ: nút "Tạo rule" chỉ hiện với `editor+`, nhưng API vẫn check.

---

## 7. Routing & Navigation

- **Route group `(dashboard)`** bọc toàn bộ trang protected bằng `AuthGuard` + `DashboardShell` (sidebar + topbar). `/login` nằm ngoài group.
- **Sidebar điều hướng** mở dần theo giai đoạn (mục 9) — chỉ hiện mục mà role + giai đoạn cho phép.
- **Workspace switcher** (GĐ3) ở topbar: đổi workspace → set `X-Workspace-ID` cho client + reset Query cache theo `workspaceId`.
- **Deep-link Grafana/Jaeger** mở tab mới với context (service, time range, trace_id) — mục 11.

---

## 8. Component & Design System

- **shadcn/ui** (style `new-york`, RSC, base `slate`) — copy-in primitive, không phải dependency runtime nặng. Tokens qua CSS variables ([globals.css](../frontend/app/globals.css)), dark mode `class`.
- **Data-table** (GĐ2): TanStack Table + shadcn — cột sortable, filter, server-side pagination (đọc/ghi URL state). Dùng cho rules, alerts, incidents, delivery history. Theo hướng dẫn `ecc:dashboard-builder` + `ecc:design-system` (KHÔNG dùng taste-skill cho dashboard — xem CLAUDE.md).
- **Component dùng chung cần chuẩn hóa:** `SeverityBadge`, `StatusPill`, `RelativeTime`, `EmptyState`, `ErrorState`, `LoadingSkeleton`, `ConfirmDialog` (cho hành động nguy hiểm: xóa rule, switch pipeline mode).
- **Loading/error/empty là bắt buộc** cho mọi view có data — không để màn trắng. Skeleton khi loading, `ErrorState` (kèm trace_id + nút retry) khi lỗi, `EmptyState` khi rỗng.

---

## 9. Màn Hình Theo Giai Đoạn (map roadmap + API)

> Đồng bộ với [12-roadmap.md](12-roadmap.md) và endpoint [07](07-api-specification.md). FE của mỗi giai đoạn làm **sau** khi API tương ứng sẵn sàng.

| GĐ | Route | Màn hình | API tiêu thụ | Ghi chú UX |
|----|-------|----------|--------------|------------|
| 1 ✅ | `/login`, `/`, `/profile` | Login, Tổng quan (skeleton), Hồ sơ | `auth/login`, `auth/me`, `auth/logout` | Đã có; widget số liệu còn `—` |
| 2 | `/alerts/rules` | Bảng rules + form tạo/sửa (PromQL + labels + runbook_url bắt buộc) | `GET/POST/PUT/DELETE alerts/rules`, `:id/state` | Validate PromQL lỗi → hiện lỗi backend tại field; báo `sync_status` |
| 2 | `/alerts/active` | Active alerts + ack + silence | `alerts/active`, `:id/acknowledge`, `:id/silence` | Optimistic ack; dialog nhập duration+reason cho silence |
| 2 | `/alerts/history` | Lịch sử (filter service/severity/time) | `alerts/history` | Filter ở URL state |
| 2 | `/logs` | Log search + filter + tail (SSE) + xem theo trace | `logs/search`, `logs/tail`, `logs/trace/:id`, `logs/stats` | Tail = SSE (mục 10); deep-link sang trace |
| 2-3 | `/pipeline` | Status, DLQ review/retry, ILM editor | `pipeline/status\|dlq\|dlq/retry\|ilm\|datastreams` | Retry/mode-switch qua `ConfirmDialog` (admin) |
| 3 | `/slo` | SLO list + define + bảng compliance + budget gauge | `slos`, `:id/budget`, `compliance` | Gauge tóm tắt; chi tiết burn-rate → Grafana |
| 3 | `/incidents` | Board (theo status) + detail + timeline + postmortem editor | `incidents*`, `:id/timeline`, `:id/postmortem`, `metrics` | Timeline append-only; postmortem có template (root cause/impact/action items) |
| 3 | `/oncall` | Lịch on-call + override | `oncall/current\|schedule\|override` | Calendar view; timezone-aware |
| 3 | `/notifications` | Channels + templates + test + history | `notifications/channels*`, `templates`, `history` | Nút "Gửi test"; secret nhập 1 chiều (write-only) |
| 3 | `/settings/workspace` | Workspace + members + RBAC | `workspaces*`, `members*` | platform_admin tạo workspace |
| 4 | `/reports`, `/topology`, `/usage` | Scheduled reports, service map, usage/cost | `reports*`, `topology`, `usage`, `export*` | Export async (202 + poll job) |

---

## 10. Realtime (SSE)

- **Log tail** (`GET /api/v1/logs/tail`, GĐ2) và **in-app notification** (incident, GĐ4) dùng **SSE** (`EventSource`), không WebSocket — một chiều server→client, đơn giản hơn, hợp HTTP/2.
- Giới hạn: tail tối đa **5 connection/workspace** ([07 §3](07-api-specification.md)) — FE đóng connection khi rời trang (`useEffect` cleanup), không mở nhiều tab tail song song không cần thiết.
- Buffer + auto-reconnect với backoff khi mất kết nối; hiển thị trạng thái "đang kết nối lại".
- Pause/resume + giới hạn dòng giữ trong DOM (virtualized list) để tránh phình bộ nhớ khi log dày.

---

## 11. Tích Hợp Grafana & Jaeger (correlation)

FE **không vẽ lại** time-series. Thay vào đó nối liền 3 trụ cột ([04](04-metrics-tracing.md)) qua deep-link:

- **Sang Grafana:** từ một service/alert → mở dashboard Grafana đúng `var-service` + time range (`from`/`to`). URL dashboard cấu hình qua env (`NEXT_PUBLIC_GRAFANA_URL`).
- **Sang trace:** từ một log dòng có `trace_id` → mở Jaeger/Grafana Traces waterfall của đúng trace.
- **Nhúng (tùy chọn):** panel Grafana nhúng qua iframe/snapshot cho widget tổng quan trên trang `/` — nhưng ưu tiên deep-link để giữ FE nhẹ.

> Lý do: trùng với ADR-005 (Grafana single pane of glass cho metrics). FE bổ trợ, không cạnh tranh.

---

## 12. Observability Của Chính FE (dogfood)

LogMon là nền tảng observability → FE cũng phải tự quan sát:

- **Web Vitals** (LCP/INP/CLS) gửi về endpoint backend (hoặc OTel browser exporter — tùy chọn GĐ4) để theo dõi UX thực tế.
- **Error boundary** ở mức route group: lỗi render → hiển thị `ErrorState` + báo cáo lỗi (kèm trace_id nếu có) về backend, không crash trắng app.
- **Không log dữ liệu nhạy cảm** ra console ở production (token, PII) — tuân [03 §3](03-logs-pipeline.md) (cùng nguyên tắc).

---

## 13. Accessibility & i18n

- **a11y (WCAG 2.2 AA):** mọi input có `<label htmlFor>`; lỗi form announce qua `aria-live`; điều hướng bàn phím đầy đủ; focus ring rõ; màu không phải kênh thông tin duy nhất (severity có cả icon + text). Review bằng `ecc:a11y-architect`; test tự động bằng `axe` (mục 14).
- **i18n:** UI tiếng Việt là mặc định (`<html lang="vi">`). Chuỗi tách ra dictionary nếu cần đa ngôn ngữ sau (YAGNI — chưa làm cho tới khi có nhu cầu thật).

---

## 14. Testing (mở rộng [11 §2.3, §3](11-coding-testing-standards.md))

| Tầng | Công cụ | Đối tượng | Gate |
|------|---------|-----------|------|
| Component/unit | Vitest + Testing Library | logic UI có nhánh: form validation, error mapping, RBAC `can()`, `withCsrf`, SSE reducer | chạy mọi PR |
| a11y | `vitest-axe` / `@axe-core/playwright` | không có vi phạm a11y nghiêm trọng trên trang chính | mọi PR |
| E2E | Playwright | login/logout, refresh hết hạn, tạo rule → thấy trong bảng, log search, ack alert | nightly/staging |
| Visual regression | Playwright snapshot (hoặc Chromatic nếu có Storybook) | layout shell, data-table, badge — chống vỡ giao diện | nightly *(GĐ3+)* |
| Performance | Lighthouse CI | budget: LCP < 2.5s, bundle JS trang chính < ngưỡng đặt | nightly *(GĐ3+)* |

> Behavior-first: test theo hành vi người dùng (query theo role/label), không test chi tiết implementation. Coverage logic UI hướng mốc chung dự án (xem [testing.md] toàn cục).

---

## 15. Performance

- **RSC mặc định** — đẩy fetch lên server, giảm JS client; `"use client"` chỉ ở component tương tác.
- **Code-split theo route** (App Router tự động) + lazy `dynamic()` cho component nặng (data-table lớn, log viewer SSE).
- **React Compiler** (khi lên React 19) tự memo — bỏ `useMemo/useCallback` thủ công trừ hot path đo được.
- **Caching:** TanStack `staleTime` hợp lý theo độ động của dữ liệu; tránh refetch thừa. Dữ liệu tĩnh (workspace list) cache dài.
- **Virtualized list** cho log tail / bảng dài.

---

## 16. Build & Deploy

- **`output: 'standalone'`** (Next config) → Docker image nhỏ (chỉ runtime cần thiết). Hiện chưa bật — thêm khi đóng gói prod (mục 17).
- **Env:** chỉ biến `NEXT_PUBLIC_*` mới lộ ra client; **không** đặt secret vào `NEXT_PUBLIC_`. `NEXT_PUBLIC_API_BASE_URL`, `NEXT_PUBLIC_GRAFANA_URL`.
- **Reverse proxy:** Nginx/Caddy route `/` → frontend, `/api` → backend, cùng origin để cookie `SameSite=Strict` hoạt động (xem [10](10-deployment-operations.md)). Tránh CORS bằng cách phục vụ cùng origin ở prod.
- **Health:** trang `/` (hoặc `/api/health` của Next) cho healthcheck compose.

---

## 17. Điểm Cần Đồng Bộ (drift hiện tại → backlog)

> Phát hiện khi đối chiếu code FE hiện tại với spec. Đưa vào story GĐ2 để hội tụ.

| # | Drift | Hiện tại (`frontend/`) | Chuẩn (doc) | Hành động |
|---|-------|------------------------|-------------|-----------|
| 1 | **Response envelope** | `{ success, data, error: string }` ([lib/api.ts](../frontend/lib/api.ts)) | `{ data, error: {code,message}, meta }` ([07 §1.1](07-api-specification.md)) | Sửa client lõi theo 07; backend đã/đang dùng `success`? → thống nhất 1 dạng, sửa cả 2 phía trong cùng PR |
| 2 | **Endpoint register** | `POST /api/v1/users` | `POST /api/v1/auth/register` | Đổi FE theo catalog |
| 3 | **Endpoint me** | `GET /api/v1/me` | `GET /api/v1/auth/me` | Đổi FE theo catalog |
| 4 | **CSRF token** | chưa gửi `X-CSRF-Token` | bắt buộc cho mutation (GĐ2) | Thêm `withCsrf()` vào client lõi |
| 5 | **Phiên bản** | Next 14.2 / React 18.3 | Next 16.2 / React 19 + Compiler | Nâng cấp + bật React Compiler |
| 6 | **API types** | viết tay trong `lib/api.ts` | sinh từ `openapi.yaml` | Thêm codegen pipeline |
| 7 | **Server-state** | raw `fetch` + `useState` | TanStack Query | Giới thiệu khi thêm màn CRUD GĐ2 |
| 8 | **`output: standalone`** | chưa bật | bật cho Docker | Thêm vào `next.config.mjs` khi đóng gói |

---

## 18. ADR Liên Quan & Đề Xuất

**Đã có (tham chiếu):** ADR-005 (Grafana single pane), ADR-023 (refresh rotation + CSRF), ADR-029 (tách demo khỏi platform).

**ADR FE — đã ghi chính thức tại [13](13-adr.md) (2026-06-23):**
- **ADR-036** (← FE-1): Next.js App Router + RSC mặc định, client chỉ ở lá tương tác.
- **ADR-037** (← FE-2): TanStack Query server-state; URL là nguồn sự thật filter/pagination.
- **ADR-038** (← FE-3): OpenAPI codegen chống drift FE↔BE (không viết type tay).
- **ADR-039** (← FE-4): Grafana/Jaeger deep-link, FE không reimplement time-series viz.

> **Cập nhật theo dõi:** mọi thay đổi lệch file này → sửa doc trong cùng PR (doc là source of truth — [11 §4](11-coding-testing-standards.md)).
