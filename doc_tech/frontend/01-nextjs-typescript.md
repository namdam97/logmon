# Next.js + TypeScript (admin dashboard)
> Module FE-1 · app router, server/client components, data fetching, auth · Độ khó: 🥉→🥇 · Prereqs: không

> **Lưu ý phiên bản (đọc trước):** Bài này dạy theo mô hình **App Router** của Next.js (ổn định từ v13, là mặc định ở v14→16). Tiêu đề module ghi "Next.js 16" là **mục tiêu** (`doc_v2/14-frontend-architecture.md` §2 ghi Next.js 16.2 + React 19 là "pinned target"). Repo LogMon hiện **đã implement Next.js 14.2.35 + React 18.3 + TypeScript 5.5+** — `frontend/package.json:18-19` khai báo `next ^14.2.5` / `react ^18.3.1` (lockfile khoá `next@14.2.35`, `react@18.3.1`), `typescript ^5.5.3` ở `package.json:32` (resolve `5.9.3`). Mọi API App Router trong bài đều đúng cho 14.2 trở lên; chỗ nào chỉ có ở 15/16 (ví dụ React Compiler) sẽ đánh dấu **(planned)**.

---

## 1. Vì sao kỹ năng này quan trọng trong LogMon

LogMon là nền tảng observability cho Go microservices. Backend (Go) gánh nghiệp vụ; **frontend Next.js là "control plane"** — nơi SRE tạo alert rule, ack cảnh báo, tra log, xem incident. Triết lý FE của dự án rất rõ ràng: **UI mỏng, có chủ đích** — phần trực quan hoá time-series nặng giao cho Grafana, Next.js chỉ làm CRUD cấu hình + workflow + những view Grafana không phục vụ tốt (`doc_v2/14-frontend-architecture.md:3`).

Vì là admin dashboard nội bộ, FE phải:
- **Tiêu thụ REST `/api/v1`** chứ không gọi thẳng Elasticsearch/Prometheus/Jaeger (backend enforce RBAC + workspace filter).
- **Bảo mật phiên đúng cách**: token nằm trong cookie HttpOnly, FE không bao giờ đọc được — chống XSS chiếm token. Đây cũng là lý do toàn bộ luồng auth (login → guard → CSRF) là code FE đầu tiên dự án viết.
- **Type-safe tuyệt đối** với backend Go: response envelope, alert rule, severity... phải khớp hợp đồng API, nếu không sẽ vỡ silently ở runtime.

Nắm chắc App Router + TypeScript là điều kiện để làm mọi màn hình GĐ2-4 (logs, SLO, incident, on-call) trong roadmap.

---

## 2. Mô hình tư duy (first principles) — giải thích từ con số 0

Hãy quên framework đi và hỏi: *một trang web cần gì?* Cần (a) lấy dữ liệu, (b) render HTML, (c) thêm tương tác (click, gõ phím). Câu hỏi cốt lõi là **việc nào chạy ở đâu** — máy chủ hay trình duyệt.

- **Server (máy chủ Node):** gần database/API, có secret, mạnh. Render ở đây → trả HTML sẵn, ít JavaScript tải về → trang nhanh, không lộ secret.
- **Client (trình duyệt):** nơi duy nhất có `useState`, `onClick`, `window`, `localStorage`. Tương tác bắt buộc ở đây.

Next.js App Router biến ý tưởng này thành quy tắc cụ thể:

1. **Thư mục `app/` = bản đồ route.** Tên thư mục = đường dẫn URL. File `page.tsx` trong thư mục `app/login/` → route `/login`. Không cần khai báo router thủ công.
2. **Mọi component mặc định là Server Component** — chạy trên server, KHÔNG vào bundle JS gửi xuống browser. Muốn biến nó thành Client Component (có hook, có sự kiện), đặt `"use client"` ở dòng đầu file.
3. **TypeScript là lớp hợp đồng.** Mọi dữ liệu qua biên (response API, props giữa component) được mô tả bằng `interface`/`type`; trình biên dịch bắt lỗi *trước khi* chạy.

Tư duy chốt: **đẩy càng nhiều việc lên server càng tốt; chỉ "leo xuống" client ở những chiếc lá thật sự cần tương tác.** Đây là nguyên tắc xuyên suốt doc_v2/14 ("RSC mặc định — `"use client"` chỉ ở lá cây tương tác", `doc_v2/14:86`).

---

## 3. Khái niệm cốt lõi (tăng dần độ khó)

### 3.1 File-based routing & layout

Trong App Router, vài tên file có ý nghĩa đặc biệt:

| File | Vai trò |
|------|---------|
| `page.tsx` | Nội dung của một route (URL truy cập được) |
| `layout.tsx` | Khung bọc quanh page + các page con; **giữ nguyên khi điều hướng giữa các con** (không re-render) |
| `loading.tsx` | UI hiện trong lúc page đang tải (Next tự bọc Suspense) — *(repo chưa dùng)* |
| `error.tsx` | Bắt lỗi render của segment, phải là Client Component — *(repo chưa dùng)* |
| `(tên)` | **Route group**: gom nhóm logic, KHÔNG xuất hiện trong URL |

LogMon dùng route group `(dashboard)` để bọc mọi trang cần đăng nhập bằng cùng một layout (sidebar + guard), trong khi `/login` nằm ngoài group.

### 3.2 Server Component vs Client Component

```tsx
// Server Component (mặc định) — chạy trên server, fetch trực tiếp, không có hook
async function ServerList() {
  const res = await fetch("https://api/.../alerts"); // chạy server-side
  const data = await res.json();
  return <ul>{data.map((a) => <li key={a.id}>{a.name}</li>)}</ul>;
}
```

```tsx
"use client"; // bắt buộc khi cần state/sự kiện
import { useState } from "react";
function Counter() {
  const [n, setN] = useState(0);
  return <button onClick={() => setN(n + 1)}>{n}</button>;
}
```

| | Server Component | Client Component |
|---|---|---|
| Mặc định? | ✅ Có | Cần `"use client"` |
| `useState`/`onClick` | ❌ Không | ✅ Có |
| Truy cập DB/secret | ✅ An toàn | ❌ Không |
| Vào bundle JS client | ❌ Không | ✅ Có (tốn JS) |
| Có thể `import` thành phần kia? | import được Client | **KHÔNG** import được Server (chỉ nhận qua `children`) |

Quy tắc vàng ([Next.js docs](https://nextjs.org/docs/app/getting-started/server-and-client-components)): **đẩy `"use client"` xuống lá cây gần tương tác nhất**, đừng "use client" cả trang.

### 3.3 Data fetching: 3 lựa chọn

1. **Server Component `async`** — `await fetch()` ngay trong component (lựa chọn mặc định, ít JS nhất).
2. **Client + `useEffect` + `fetch`** — gọi sau khi mount; phù hợp khi cần cookie của trình duyệt hoặc tương tác. **Đây là cách LogMon hiện dùng** (vì luồng auth dựa trên cookie và phần lớn trang là `"use client"`).
3. **TanStack Query** *(planned, doc_v2/14 §4.3)* — thư viện server-state: cache, refetch nền, `invalidateQueries`, optimistic update. Là mục tiêu GĐ2 nhưng **chưa có trong repo**.

### 3.4 TypeScript ở biên hệ thống

```ts
export interface Envelope<T> {  // hợp đồng response chung
  success: boolean;
  data: T | null;
  error?: string;
}
```

Generic `<T>` tái dùng envelope cho mọi kiểu (`User`, `ActiveAlert[]`...). `strict: true` (`tsconfig.json:7`) ép xử lý `null` (`data: T | null`) và cấm `any` ngầm — đúng [TypeScript strict mode](https://www.typescriptlang.org/tsconfig/strict.html).

---

## 4. LogMon dùng nó thế nào (bám code thật — path:line, ghi rõ implemented/planned)

**Cấu trúc route (implemented):**
- `frontend/app/layout.tsx:9` — root layout: `<html lang="vi">`, set `metadata` (Server Component, không có `"use client"`).
- `frontend/app/login/page.tsx:1` — trang `/login` công khai, là **Client Component** (`"use client"`) vì có form + `useState` + `useRouter`.
- `frontend/app/(dashboard)/layout.tsx:1` — route group `(dashboard)`: bọc con bằng `<AuthGuard>` + `<DashboardShell>`.
- `frontend/app/(dashboard)/page.tsx` → `/` (Tổng quan), `.../alerts/page.tsx` → `/alerts`, `.../profile/page.tsx` → `/profile`.

**Bảo vệ route — pattern client-side guard (implemented):**
`frontend/components/auth-guard.tsx:19-27` gọi `me()` trong `useEffect`; lỗi → `router.replace("/login")`. Có cờ `active` để chặn `setState` sau khi unmount (tránh race). doc_v2/14 §6.2 ghi rõ đây là cách GĐ1; **Next.js middleware ở edge là (planned) GĐ2+** để chặn flash nội dung protected.

**HTTP client lõi (implemented):** `frontend/lib/api.ts` là **điểm duy nhất chạm `fetch`**:
- `request<T>()` (dòng 27-40): set `credentials: "include"` để gửi cookie HttpOnly qua origin; parse `Envelope<T>`; ném `Error` khi `!res.ok || !body.success`.
- `mutate<T>()` (dòng 57-70): với POST/PUT/DELETE, đọc cookie `lm_csrf` qua `readCookie()` (dòng 44-51) và đính header `X-CSRF-Token` — đây là **CSRF double-submit**, khớp với commit `43b6725` ở backend.
- Các hàm domain: `login`/`me`/`logout` (dòng 82-99), `listActiveAlerts`/`acknowledgeAlert`/`listAlertRules`/`setAlertRuleEnabled` (dòng 133-154). Type `ActiveAlert`/`AlertRule` (dòng 104-130) **viết tay** — doc_v2/14 §17 drift #6 ghi mục tiêu là sinh từ OpenAPI codegen **(planned)**.

**Trang Alerts — pattern Client Component có state phức tạp (implemented):** `frontend/app/(dashboard)/alerts/page.tsx` minh hoạ chuẩn xử lý data ở client: `useState` cho `alerts`/`rules`/`loading`/`error`, `useRef(mounted)` (dòng 62-68) chặn setState sau unmount, `Promise.all([...])` (dòng 74) fetch song song, optimistic-ish update bằng `setAlerts((prev) => prev.map(...))` (dòng 97) — cập nhật **bất biến** đúng coding-style của dự án.

**Same-origin & proxy (implemented):** `frontend/next.config.mjs:12-16` rewrite `/api/:path*` → `http://localhost:8080` cho dev/e2e, để trình duyệt thấy FE và API **cùng origin** — điều kiện để CSRF double-submit (JS đọc được cookie `lm_csrf`) hoạt động.

**Khoảng trống đã biết:** trang Tổng quan `frontend/app/(dashboard)/page.tsx:12-17` còn để số liệu `"—"` ("chưa nối API metrics... GĐ sau"). TanStack Query, react-hook-form+zod, OpenAPI codegen, `output: 'standalone'`, middleware đều là **(planned)** theo bảng drift `doc_v2/14:306-316`.

---

## 5. Best practices (mỗi mục kèm 1 nguồn đã research)

1. **Server Component mặc định, `"use client"` chỉ ở lá tương tác.** Giảm JS gửi về client → trang nhanh hơn. ([Next.js — Server and Client Components](https://nextjs.org/docs/app/getting-started/server-and-client-components))
2. **Fetch ở server, truyền xuống client qua props** để tránh "API ping-pong"; client chỉ fetch khi cần cookie/tương tác. ([Next.js docs, mục trên](https://nextjs.org/docs/app/getting-started/server-and-client-components))
3. **Token phiên trong cookie HttpOnly, KHÔNG localStorage.** localStorage đọc được bởi mọi JS (kể cả XSS); HttpOnly thì không. LogMon đã làm đúng (`lib/api.ts:31` `credentials: "include"`). ([Authgear — Next.js Session Management](https://www.authgear.com/post/nextjs-session-management/))
4. **Coi middleware là routing, KHÔNG phải biên bảo mật — luôn verify lại ở API/Server Action.** CVE-2025-29927 (CVSS 9.1, 03/2025) cho phép bypass toàn bộ middleware qua header `x-middleware-subrequest`; fix ở 14.2.25+. Repo đang ở 14.2.35 → đã vá. ([Next.js Security Best Practices 2026 — Authgear](https://www.authgear.com/post/nextjs-security-best-practices/))
5. **Dùng `loading.tsx`/Suspense + `error.tsx` cho mọi view có data** — không để màn trắng; streaming cho phép hiện phần nhanh trước. ([Next.js — Loading UI and Streaming](https://nextjs.org/docs/14/app/building-your-application/routing/loading-ui-and-streaming))
6. **Bật `strict: true`, cấm `any`, dùng `unknown` + type guard khi cần linh hoạt.** Strict mode giảm đáng kể bug type ra production. ([TypeScript — tsconfig strict](https://www.typescriptlang.org/tsconfig/strict.html))
7. **TanStack Query cho server-state khi app lớn** *(planned)*: `useQuery`/`useMutation` + `invalidateQueries`, đặt `QueryClientProvider` trong một client component. ([TanStack Query — Next.js example](https://tanstack.com/query/v5/docs/framework/react/examples/nextjs))

---

## 6. Lỗi thường gặp & anti-patterns

- **`"use client"` ở đỉnh cây (root layout / page lớn):** kéo cả nhánh xuống client, mất lợi ích RSC. Sửa: đặt `"use client"` ở component lá. *(LogMon chấp nhận client-heavy ở GĐ1 vì auth dựa cookie; sẽ tinh chỉnh GĐ2+.)*
- **Lưu token vào `localStorage`/`sessionStorage`:** mở cửa cho XSS chiếm phiên. Luôn dùng cookie HttpOnly + `credentials: "include"`.
- **Tin middleware là tường lửa:** xem best practice #4 (CVE-2025-29927). Backend Go của LogMon vẫn verify mọi request — đúng "defense in depth" (`doc_v2/14:188`).
- **Quên `key` ổn định khi `.map()` render list:** dùng `a.id` (như `alerts/page.tsx:171`), không dùng index → tránh bug reconciliation.
- **Mutate state trực tiếp:** `arr.push(x); setArr(arr)` → React không re-render. Phải tạo bản mới: `setAlerts((prev) => prev.map(...))` (đúng nguyên tắc bất biến trong CLAUDE.md).
- **`setState` sau khi component unmount:** rò rỉ + warning. LogMon chống bằng cờ `active`/`mounted.current` (`auth-guard.tsx:24`, `alerts/page.tsx:62`).
- **Dùng `any` để "cho qua" lỗi TS:** mất toàn bộ type-safety. Ưu tiên `unknown` + thu hẹp kiểu, hoặc khai báo `interface` đúng (như `Envelope<T>`).
- **Viết tay type API rồi để lệch backend:** drift âm thầm. doc_v2/14 §17 đặt mục tiêu OpenAPI codegen để loại bỏ.

---

## 7. Lộ trình luyện tập NGAY trong repo LogMon

### 🥉 Cơ bản
1. Chạy `make dev-fe` (hoặc `cd frontend && pnpm dev`), mở `http://localhost:3000/login`, đăng nhập và quan sát điều hướng `/login → /` do `router.replace("/")` ở `login/page.tsx:31`.
2. Thêm một mục nav mới (ví dụ "Logs", icon `lucide-react`) vào mảng `NAV` trong `frontend/components/layout/dashboard-shell.tsx:11`, rồi tạo `app/(dashboard)/logs/page.tsx` tối thiểu trả về `<h1>Logs</h1>` — xác nhận route group tự áp dụng sidebar.
3. Trong `frontend/app/(dashboard)/page.tsx:12`, đổi một giá trị `STATS` từ `"—"` thành chuỗi tĩnh và xem hot-reload cập nhật card.
4. Thêm `interface` mới vào `frontend/lib/api.ts` (ví dụ `SiloStat`) và để `tsc` (chạy `pnpm build`) bắt lỗi nếu thiếu field — cảm nhận strict mode.

### 🥈 Trung cấp
1. Viết hàm `createSilence(matchers, duration)` trong `frontend/lib/api.ts` dùng `mutate<T>()` cho endpoint silence có thật ở backend — `POST /api/v1/alerts/silences` (matcher-based, không phải theo từng instance id; xem `backend/internal/alerting/adapters/http/silence_handler.go:54`), kèm test trong `frontend/lib/api.test.ts` theo pattern AAA + mock `fetch` (đối chiếu test `acknowledgeAlert` dòng 62).
2. Thêm trạng thái **empty/error rõ ràng** cho trang Tổng quan giống cách `alerts/page.tsx:139-143` render `<p role="alert">Lỗi: ...</p>`.
3. Tách logic fetch của `alerts/page.tsx` ra một custom hook `useAlerts()` trong `frontend/lib` (giữ `useRef(mounted)`), trang chỉ gọi hook — bước đệm tiến tới feature-folder của doc_v2/14 §3.
4. Viết một e2e Playwright mới trong `frontend/e2e/` cho luồng "ack một alert đang firing → badge đổi sang `acknowledged`" (mẫu: `e2e/auth.spec.ts`).
5. Bổ sung file `app/(dashboard)/loading.tsx` trả skeleton, kiểm tra Next tự bọc Suspense khi chuyển trang.

### 🥇 Nâng cao
1. **Giới thiệu TanStack Query (planned → thực thi):** thêm `@tanstack/react-query`, tạo `QueryClientProvider` trong một client component đặt ở `app/layout.tsx`, chuyển `alerts/page.tsx` sang `useQuery`/`useMutation` + `invalidateQueries`, giữ `lib/api.ts` làm `queryFn`.
2. **Thêm Next.js middleware** kiểm tra sự hiện diện cookie phiên ở edge để redirect *trước khi* render (doc_v2/14 §6.2) — và viết note bảo mật rằng middleware KHÔNG thay thế guard server (CVE-2025-29927).
3. **Khắc phục drift envelope #1** (`doc_v2/14:308`): refactor `Envelope<T>` từ `{success,data,error:string}` sang `{data, error:{code,message}, meta}` đồng bộ cả `lib/api.ts` + `lib/api.test.ts`, map `error.code` → xử lý UI (401 refresh, 403 toast).
4. **Bật `output: 'standalone'`** trong `next.config.mjs` rồi build Docker image, đo kích thước — drift #8.
5. Thêm `vitest-axe` cho một trang và sửa vi phạm a11y (doc_v2/14 §14).

---

## 8. Skill/agent ECC nên dùng khi luyện

- **`ecc:nextjs-turbopack`** — khi cấu hình/chạy dev server, debug build, bật Turbopack hoặc tối ưu thời gian build/HMR cho `frontend/`.
- **`ecc:react-patterns`** — khi tách custom hook (`useAlerts`), quyết định ranh giới Server/Client Component, hoặc cấu trúc feature-folder GĐ2.
- **`ecc:react-review`** (kéo theo `ecc:typescript-reviewer` trên file `.tsx`) — chạy **ngay sau khi viết/sửa component**: bắt lỗi hook, render thừa, ranh giới server/client, a11y. Khớp rule "code-reviewer ngay sau khi viết code" của dự án.
- **`ecc:typescript-reviewer`** — khi refactor type API (drift #6/#1), kiểm tra không còn `any`, generic dùng đúng, `null`-safety.
- **`ecc:react-build`** — khi build FE fail (lỗi JSX/TSX, hydration mismatch, ranh giới server/client): sửa tối thiểu, an toàn.
- Bổ trợ thiết kế: **`ecc:dashboard-builder` + `ecc:design-system`** cho data-table GĐ2 (CLAUDE.md nhấn mạnh KHÔNG dùng `taste-skill` cho dashboard).

---

## 9. Tài nguyên học thêm (link đã research, có chú thích 1 dòng)

- [Next.js — Server and Client Components](https://nextjs.org/docs/app/getting-started/server-and-client-components) — tài liệu chính thức về khi nào dùng RSC vs `"use client"`.
- [Next.js — Loading UI and Streaming](https://nextjs.org/docs/14/app/building-your-application/routing/loading-ui-and-streaming) — `loading.tsx`, Suspense, streaming (đúng nhánh 14.x của repo).
- [Authgear — Next.js Security Best Practices (2026)](https://www.authgear.com/post/nextjs-security-best-practices/) — cookie HttpOnly, CSRF, và phân tích CVE-2025-29927 middleware bypass.
- [Authgear — Next.js Session Management](https://www.authgear.com/post/nextjs-session-management/) — vì sao session token nên ở cookie HttpOnly chứ không localStorage.
- [TanStack Query — Next.js example (v5)](https://tanstack.com/query/v5/docs/framework/react/examples/nextjs) — mẫu setup server-state cho App Router (mục tiêu GĐ2 của LogMon).
- [TypeScript — tsconfig `strict`](https://www.typescriptlang.org/tsconfig/strict.html) — chi tiết các cờ strict mode mà `frontend/tsconfig.json` đang bật.

---

## 10. Checklist "đã hiểu"

- [ ] Giải thích được vì sao component mặc định là Server Component và khi nào phải thêm `"use client"`.
- [ ] Chỉ ra trong repo đâu là Server Component, đâu là Client Component, và vì sao (`app/layout.tsx` vs `login/page.tsx`).
- [ ] Hiểu route group `(dashboard)` ảnh hưởng URL và layout thế nào, khác `/login` ra sao.
- [ ] Mô tả luồng auth LogMon: `AuthGuard` → `me()` → redirect, và cookie HttpOnly + CSRF double-submit qua `mutate()`.
- [ ] Biết tại sao token phiên KHÔNG để localStorage, và middleware KHÔNG phải biên bảo mật (CVE-2025-29927).
- [ ] Phân biệt được phần đã implement (Next 14.2, raw fetch + useState) vs planned (Next 16/React 19, TanStack Query, OpenAPI codegen, middleware).
- [ ] Viết được một hàm API mới trong `lib/api.ts` kèm test mock `fetch`, cập nhật state theo kiểu bất biến.
- [ ] Đọc `tsconfig.json` và nói được `strict: true` bắt loại lỗi nào ở runtime.
