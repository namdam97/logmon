# TailwindCSS + shadcn/ui design system
> Module FE-2 · utility-first, cva/clsx/tailwind-merge, component pattern, a11y · Độ khó: 🥉→🥇 · Prereqs: FE-1

## 1. Vì sao kỹ năng này quan trọng trong LogMon

LogMon là một **admin control plane** cho observability, không phải landing page. Theo `doc_v2/14-frontend-architecture.md` §1, Next.js chỉ làm CRUD cấu hình + workflow SRE (alert rules, ack, silence, incident board, log search), còn time-series chart nặng giao cho Grafana. Hệ quả: 90% UI là **bảng dữ liệu, badge trạng thái, form cấu hình** — đúng địa hạt mà TailwindCSS + shadcn/ui giải quyết tốt nhất.

Cụ thể trong repo hiện tại (`frontend/`):
- Trang `/alerts` (`frontend/app/(dashboard)/alerts/page.tsx`) đã render 2 bảng thật: active alerts + alert rules, với badge severity/status đổi màu theo dữ liệu.
- Toàn bộ primitive (`Button`, `Card`, `Table`, `Badge`, `Input`, `Label`) nằm trong `frontend/components/ui/` — **copy-in**, không phải dependency runtime.

Nắm vững mảng này = bạn sửa được layout shell, thêm cột bảng, đổi màu severity, đảm bảo a11y (WCAG 2.2 AA là yêu cầu trong `doc_v2/14` §13) mà không phá vỡ design system. Đây là kỹ năng dùng hằng ngày khi build các trang GĐ2/GĐ3 (logs, slo, incidents — hiện **planned**).

## 2. Mô hình tư duy (first principles) — giải thích từ con số 0

Bắt đầu từ vấn đề gốc: **làm sao style một UI nhất quán mà không rơi vào hỗn loạn CSS?**

- **CSS truyền thống**: bạn viết file `.css`, đặt tên class (`.alert-badge--critical`), rồi cố nhớ class nào còn dùng. Theo thời gian sinh ra "CSS chết" và xung đột specificity.
- **Utility-first (TailwindCSS)**: thay vì đặt tên, bạn ghép các **class nguyên tử** có sẵn ngay trong markup — `px-3`, `text-sm`, `bg-destructive`. Mỗi class làm đúng một việc. Không còn đặt tên, không còn CSS chết: class nào không xuất hiện trong markup thì Tailwind không build ra (xem `content` trong `frontend/tailwind.config.ts:5`).

Nhưng utility-first đẻ ra 2 vấn đề mới, và đây là chỗ stack LogMon ghép vào:

1. **Markup dài và lặp** → giải bằng **component**. Gom chuỗi class vào một React component (`Button`) để tái sử dụng.
2. **Một component có nhiều biến thể** (button default/destructive/outline; badge critical/warning/info) → giải bằng **cva (class-variance-authority)**: khai báo bảng "prop → chuỗi class" một cách type-safe.

Cuối cùng, khi caller truyền thêm `className` để override, hai class Tailwind có thể đụng nhau (`px-2` vs `px-4`). Trình duyệt chọn class **đứng sau trong CSS**, không phải class viết sau trong chuỗi → kết quả khó đoán. **tailwind-merge** giải đúng việc này: hợp nhất, để class sau thắng class trước một cách có ngữ nghĩa Tailwind. `clsx` thì lo phần ghép class **có điều kiện**. Hàm `cn()` (`frontend/lib/utils.ts:5`) gói cả hai:

```ts
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}
```

Và **shadcn/ui** không phải thư viện — nó là một **bộ source code mẫu** bạn copy vào `components/ui/` rồi sở hữu hoàn toàn. Mỗi primitive là một file React + cva + `cn()`. Đó là lý do bạn thấy code đầy đủ trong repo chứ không phải import từ `node_modules`.

## 3. Khái niệm cốt lõi (tăng dần độ khó)

### 3.1 Design tokens qua CSS variables (🥉)

shadcn/ui không hardcode màu. Nó định nghĩa **token ngữ nghĩa** (semantic) dạng cặp `background`/`foreground` trong `frontend/app/globals.css:6-25`:

```css
:root {
  --primary: 222.2 47.4% 11.2%;
  --primary-foreground: 210 40% 98%;
  --destructive: 0 84.2% 60.2%;
  ...
}
.dark { --primary: 210 40% 98%; ... }   /* override cùng token */
```

`tailwind.config.ts:14-17` map token này thành utility: `bg-primary`, `text-primary-foreground`. Giá trị viết dạng **bare HSL** (`222.2 47.4% 11.2%`, không bọc `hsl()`) để hỗ trợ opacity modifier như `bg-primary/90`. Dark mode = override cùng tên token trong `.dark`, không viết lại class.

> **Ngữ nghĩa, không phải màu**: bạn dùng `bg-destructive` (ý nghĩa: nguy hiểm) chứ không `bg-red-600`. Đổi theme = đổi 1 biến, mọi nơi cập nhật.

### 3.2 cva — bảng biến thể type-safe (🥈)

Xem `frontend/components/ui/button.tsx:6-33`. `cva(base, config)` nhận:

| Tham số | Vai trò | Ví dụ trong repo |
|---------|---------|------------------|
| `base` | class luôn áp dụng | `inline-flex items-center ... rounded-md` |
| `variants` | nhóm prop → class | `variant: {default, destructive, outline...}`, `size: {sm, lg, icon}` |
| `defaultVariants` | giá trị mặc định | `{variant: "default", size: "default"}` |
| `compoundVariants` | class khi nhiều prop cùng đúng | (chưa dùng trong repo) |

`VariantProps<typeof buttonVariants>` tự suy ra type cho props — bạn không khai báo tay union `"default" | "destructive" | ...`. Đây là điểm "type-safe": gõ sai `variant="danger"` → lỗi compile.

### 3.3 cn() — thứ tự merge quyết định kết quả (🥈)

`buttonVariants({ variant, size, className })` sinh chuỗi, rồi `cn()` merge. Thứ tự chuẩn (best practice): **base → variant → className của caller (đứng cuối, thắng)**. Nhờ tailwind-merge, `<Button className="px-8">` ghi đè `px-4` mặc định mà không cần `!important`.

### 3.4 Compound component — Card & Table (🥈)

`Card` (`frontend/components/ui/card.tsx`) không phải một component khổng lồ mà là **họ component nhỏ**: `Card`, `CardHeader`, `CardTitle`, `CardDescription`, `CardContent`. Tương tự `Table` tách thành `TableHeader/Body/Row/Head/Cell`. Lợi ích: cấu trúc HTML đúng semantic (`<th scope="col">` ở `table.tsx:59`), người dùng tự compose layout, mỗi phần vẫn nhận `className` để override.

### 3.5 Data table: primitive vs headless engine (🥇)

Phân biệt rõ:

| | `Table` primitive (đã có) | Data-table (planned, GĐ2) |
|---|---|---|
| Là gì | Wrapper style cho `<table>` | TanStack Table + primitive |
| Lo việc | Markup + style | sort, filter, pagination, row selection |
| Trạng thái | Không | `SortingState`, `ColumnFiltersState`... |
| Trong repo | `frontend/components/ui/table.tsx` | chưa có — xem `doc_v2/14` §8 |

TanStack Table là **headless**: nó tính toán logic bảng (qua `useReactTable`, `getCoreRowModel()`, `getSortedRowModel()`), còn render giao cho primitive `Table` của shadcn. Cột khai báo bằng `ColumnDef<T>[]` (`accessorKey`, `header`, `cell`).

## 4. LogMon dùng nó thế nào (bám code thật — path:line, ghi rõ implemented/planned)

**Implemented (đọc thấy code):**

- **cn() helper** — `frontend/lib/utils.ts:5`. Mọi primitive đều gọi.
- **Token + dark mode** — `frontend/app/globals.css:6-45` (`:root` + `.dark`), map ở `frontend/tailwind.config.ts:8-43`. `darkMode: ["class"]` đã bật (`tailwind.config.ts:4`) **nhưng chưa có theme toggle** — chưa component nào set class `.dark` (planned).
- **cva primitives** — `button.tsx:6` (6 variant × 4 size), `badge.tsx:6` (4 variant). `Card/Table/Input/Label` không dùng cva (không có biến thể) mà chỉ `cn()`.
- **shadcn config** — `frontend/components.json`: style `new-york`, `rsc: true`, base color `slate`, alias `@/components`, `@/lib/utils`.
- **Mapping dữ liệu → token ngữ nghĩa** — `frontend/app/(dashboard)/alerts/page.tsx:32-43`: hàm `severityVariant()`/`statusVariant()` ánh xạ `critical→destructive`, `acknowledged→secondary`. Đây là pattern chuẩn: logic nghiệp vụ chọn **biến thể ngữ nghĩa**, không chọn màu trực tiếp.
- **Table thật + a11y** — `alerts/page.tsx:160-197` dùng `Table/TableHeader/TableRow/TableHead/TableCell`; nút có `aria-label` động (`:184-188`); lỗi có `role="alert"` (`:140`); `<time dateTime>` (`:177`).
- **Layout shell** — `frontend/components/layout/dashboard-shell.tsx`: sidebar dùng `cn()` cho active state (`:46-51`), `aria-current="page"` (`:45`), icon từ `lucide-react`. Responsive `hidden md:flex` (`:33`).
- **Overview cards** — `frontend/app/(dashboard)/page.tsx:29-41`: grid responsive `sm:grid-cols-2 lg:grid-cols-4`, giá trị còn `"—"` (chưa nối API metrics — **planned**).

**Planned (chỉ trong doc_v2/roadmap, code chưa có):**
- **Data-table (TanStack Table)** — `doc_v2/14` §8 dòng 204: sortable + filter + server-side pagination + URL state. Dùng cho rules/alerts/incidents/delivery history. **Chưa cài** `@tanstack/react-table` (không có trong `package.json`).
- **TanStack Query, react-hook-form + zod, codegen từ openapi** — `doc_v2/14` §2 đánh dấu ❌ chưa có; hiện trang dùng raw `fetch` + `useState` (xem `alerts/page.tsx:51-58`).
- **Nâng Next.js 16.2 / React 19 + React Compiler** — `doc_v2/14` §2; repo đang Next 14.2.5 / React 18.3.1.
- **Test a11y bằng axe** — `doc_v2/14` §14; hiện có Vitest + Playwright, **chưa có** `@axe-core/playwright`.
- **Các trang FE `slo/incident/notification`** — chưa có UI: `/incident` + `/notification` là GĐ3 (CLAUDE.md bảng BC), `/slo` là GĐ3 (`doc_v2/14` §9 dòng 222). Backend: `incident`/`notification` BC chưa có code; `slo` mới có lớp `domain/` (`backend/internal/slo/domain/`), chưa có `app/ports/adapters` và chưa có FE.

## 5. Best practices (mỗi mục kèm 1 nguồn đã research)

1. **Định nghĩa cva variants ngoài component** — để không tạo lại bảng mỗi lần render. Repo làm đúng: `buttonVariants` khai báo ở module scope (`button.tsx:6`), không trong thân `Button`. Nguồn: [cva docs](https://cva.style/docs).
2. **Thứ tự merge: base → variant → className caller** — class của caller phải đứng cuối để override; tailwind-merge xử lý xung đột. Repo: `cn(buttonVariants({ variant, size, className }))` (`button.tsx:43`). Nguồn: [Patterns for composable tailwindcss styles — Typeonce](https://www.typeonce.dev/article/patterns-for-composable-tailwindcss-styles).
3. **Theme qua CSS variables, đừng hardcode màu** — override token trong `.dark` thay vì rải `dark:bg-red-600`. Repo: `globals.css`. Nguồn: [shadcn/ui Theming](https://ui.shadcn.com/docs/theming).
4. **Data-table: tách `columns.tsx` khỏi `data-table.tsx`, dùng `ColumnDef` + `useReactTable`** — server-side thì bật `manualPagination/manualSorting` và đẩy `page/sort/filter` lên API. Nguồn: [Data Table — shadcn/ui](https://ui.shadcn.com/docs/components/radix/data-table).
5. **Bảng sortable phải accessible**: nút `<button>` bên trong `<th>`, set `aria-sort` trên header đang sort, thông báo thay đổi qua `aria-live`. Nguồn: [Sortable Table Example — W3C WAI-ARIA APG](https://www.w3.org/WAI/ARIA/apg/patterns/table/examples/sortable-table/) và [Adrian Roselli — Sortable Table Columns](https://adrianroselli.com/2021/04/sortable-table-columns.html).
6. **Màu không phải kênh thông tin duy nhất** — severity phải kèm text/icon (WCAG 1.4.1). Repo: badge luôn render cả nhãn text (`alerts/page.tsx:174`). Nguồn: `doc_v2/14` §13 + [WCAG 2.2 tables guide — TheWCAG](https://www.thewcag.com/examples/tables).

## 6. Lỗi thường gặp & anti-patterns

- **Nối chuỗi class bằng template literal thay vì `cn()`** → xung đột không được merge, override im lặng thất bại. Luôn qua `cn()`.
- **Tự thêm `className` prop tùy tiện làm trôi design system** — chỉ override khi thật cần; thêm variant mới vào cva tốt hơn là rắc `className` khắp nơi (xem khuyến nghị trong [components.build — Styling](https://www.components.build/styling)).
- **Hardcode `bg-slate-900` thay vì `bg-background`** → dark mode vỡ vì không qua token.
- **Định nghĩa `cva(...)` bên trong component** → tạo lại mỗi render, mất tối ưu.
- **Dùng `role="table"` thủ công thay vì `<table>`** → mất semantic miễn phí; W3C khuyến cáo ưu tiên HTML native.
- **Bảng sort chỉ dùng `<div onClick>`** → không focus được bằng bàn phím, thiếu `aria-sort`. Phải là `<button>` trong `<th>`.
- **Quên `key` ổn định khi map row** (repo dùng `a.id`/`r.id`, đúng — `alerts/page.tsx:171,227`).
- **Đặt secret vào `NEXT_PUBLIC_*`** — chỉ biến này lộ ra client (`doc_v2/14` §16).
- **Tự build lại time-series chart** thay vì deep-link Grafana — vi phạm ranh giới FE (`doc_v2/14` §1, ADR-005).

## 7. Lộ trình luyện tập NGAY trong repo LogMon (🥉 → 🥈 → 🥇)

### 🥉 Cơ bản
1. Thêm size `xl` (`h-12 px-10 text-base`) vào `variants.size` của `frontend/components/ui/button.tsx`, dùng thử trên nút "Làm mới" ở `alerts/page.tsx:134`.
2. Thêm token màu `warning`/`warning-foreground` (vàng amber) vào `globals.css` (cả `:root` và `.dark`) + map trong `tailwind.config.ts`, rồi cho `severityVariant("warning")` ở `alerts/page.tsx:33` trả về một variant dùng token mới.
3. Đổi `Card` overview (`page.tsx:31`) thêm `hover:shadow-md transition-shadow`, kiểm tra `cn()` không gây xung đột với `shadow` mặc định ở `card.tsx:12`.
4. Đọc `frontend/app/login/page.tsx:49,60` để thấy mẫu `<Label htmlFor>` đã nối đúng với `<Input id>` (a11y chuẩn). Áp lại đúng mẫu này khi thêm một trường mới (ví dụ ô "Workspace") — mỗi `<Input>` phải có `id` khớp `htmlFor` của `<Label>`.

### 🥈 Trung cấp
1. Thêm variant `success` vào `badge.tsx` (cva) và một icon `lucide-react` đứng cạnh text trong badge status ở bảng alerts (đảm bảo màu + icon, không chỉ màu — WCAG 1.4.1).
2. Trích cột severity/status thành component `SeverityBadge`/`StatusBadge` trong `frontend/components/` (DRY), dùng lại ở cả 2 bảng của `alerts/page.tsx`.
3. Thêm tiêu đề bảng accessible: bọc text trong `<th>` (`TableHead`) bằng `<button>` có `aria-sort` (tạm fake state sort client-side cho bảng rules), thông báo qua `aria-live`.
4. Tạo `ThemeToggle` đặt vào header `dashboard-shell.tsx:62`: toggle class `.dark` trên `document.documentElement`, xác nhận token đảo màu đúng (dark mode đã bật ở config nhưng chưa có UI bật).

### 🥇 Nâng cao
1. Cài `@tanstack/react-table`, tạo `frontend/features/alerts/components/RuleTable.tsx` (cấu trúc `columns.tsx` + `data-table.tsx`) thay bảng rules thủ công bằng data-table sortable + filter client-side (theo `doc_v2/14` §8).
2. Nâng lên **server-side**: bật `manualPagination`/`manualSorting`, đẩy `page/sort/filter` vào query string (URL là nguồn sự thật — `doc_v2/14` dòng 165) và gọi `listAlertRules` với tham số tương ứng.
3. Viết test `@axe-core/playwright` cho `/alerts` đảm bảo bảng + badge không có vi phạm a11y nghiêm trọng (đáp ứng gate `doc_v2/14` §14), và Playwright snapshot chống vỡ giao diện shell.

## 8. Skill/agent ECC nên dùng khi luyện

- **`ecc:design-system`** — khi thêm/sửa token, variant cva, hoặc chuẩn hóa primitive trong `components/ui/`. Dùng cho task 🥉#2, 🥈#1.
- **`ecc:dashboard-builder`** — khi build data-table và layout dashboard (đúng khuyến nghị `doc_v2/14` §8 cho phần data-table; **không** dùng `taste-skill` cho dashboard — xem CLAUDE.md). Dùng cho task 🥇#1, #2.
- **`ecc:a11y-architect`** (a11y/`ecc:accessibility`) — khi làm sortable header, theme toggle, label, kiểm WCAG 2.2 AA. Dùng cho task 🥈#3, #4, 🥇#3.
- **`ecc:frontend-patterns`** (`ecc:react-patterns`) — khi tách component (SeverityBadge), tổ chức feature folder, tránh re-render. Dùng cho task 🥈#2.
- **`ecc:react-review`** — chạy ngay sau khi viết component để bắt lỗi hook/render/a11y trước khi commit.

## 9. Tài nguyên học thêm (link đã research, có chú thích 1 dòng)

- [TailwindCSS — Theming/CSS variables (shadcn Theming)](https://ui.shadcn.com/docs/theming) — cách token ngữ nghĩa + dark mode override hoạt động (chính xác mô hình repo dùng).
- [cva — class-variance-authority docs](https://cva.style/docs) — API `cva()`, `VariantProps`, định nghĩa variants ngoài component.
- [Data Table — shadcn/ui (TanStack Table)](https://ui.shadcn.com/docs/components/radix/data-table) — `ColumnDef`, `useReactTable`, manual vs auto pagination/sorting.
- [Patterns for composable tailwindcss styles — Typeonce](https://www.typeonce.dev/article/patterns-for-composable-tailwindcss-styles) — thứ tự merge base→variant→override và clsx/tailwind-merge.
- [Sortable Table Example — W3C WAI-ARIA APG](https://www.w3.org/WAI/ARIA/apg/patterns/table/examples/sortable-table/) — pattern chuẩn `aria-sort` + `<button>` trong `<th>`.
- [Adrian Roselli — Sortable Table Columns](https://adrianroselli.com/2021/04/sortable-table-columns.html) — sâu về accessible sortable, các bẫy thực tế.
- [shadcn/ui Theming Best Practices: CSS Variables vs Tailwind Config](https://www.paulserban.eu/blog/post/shadcnui-theming-best-practices-css-variables-vs-tailwind-config/) — khi nào dùng CSS var vs config.

## 10. Checklist "đã hiểu" (self-assessment)

- [ ] Giải thích được vì sao `cn()` cần cả `clsx` và `tailwind-merge` (điều kiện + giải xung đột), và chỉ ra file `lib/utils.ts:5`.
- [ ] Phân biệt được token ngữ nghĩa (`bg-destructive`) với màu thô (`bg-red-600`) và biết dark mode hoạt động bằng cách override token trong `.dark`.
- [ ] Thêm được một `variant` mới vào `button.tsx`/`badge.tsx` qua cva mà type tự suy ra đúng.
- [ ] Biết vì sao class `className` của caller phải đứng cuối trong `cn()` để override.
- [ ] Phân biệt `Table` primitive (style) vs data-table (TanStack engine — sort/filter/paginate), và biết phần nào implemented vs planned trong LogMon.
- [ ] Chỉ ra cách `alerts/page.tsx` map dữ liệu nghiệp vụ → variant ngữ nghĩa thay vì chọn màu trực tiếp.
- [ ] Nêu được 3 yêu cầu a11y cho bảng sortable: `<button>` trong `<th>`, `aria-sort`, thông báo `aria-live`; và vì sao màu không được là kênh thông tin duy nhất.
- [ ] Biết ranh giới FE LogMon: không reimplement Grafana, không gọi datastore trực tiếp.
