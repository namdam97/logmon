---
description: Vòng browser-QA thường trú cho frontend — lái ecc:e2e-runner chạy Playwright, phân loại lỗi, đề xuất fix tối thiểu.
argument-hint: "[luồng cần kiểm, vd: auth | dashboard | alerts] (bỏ trống = toàn bộ e2e)"
allowed-tools: Bash, Read, Grep, Glob, Task
---

# /qa-fe — Browser-QA thường trú (FE)

Mục tiêu: biến QA trình duyệt thành **nghi thức lặp lại được**, không phải chạy rời rạc.
Lõi dùng agent `ecc:e2e-runner` (Playwright + system Chrome) — KHÔNG tự cài framework mới.

## Quy trình

1. **Xác định phạm vi** từ `$ARGUMENTS` (mặc định: toàn bộ `frontend/e2e/*.spec.ts`).
2. **Dựng stack tối thiểu** giống `make e2e`: backend (`userservice` + Postgres) ở `:8080`,
   FE `next build` + `next start` ở `:3000`. Nếu stack đã chạy thì tái dùng.
3. **Giao cho `ecc:e2e-runner`**:
   - chạy `pnpm exec playwright test` (lọc theo phạm vi nếu có),
   - thu artifact (screenshot/trace) cho test fail,
   - **phân loại**: lỗi thật (app bug) vs flaky vs lỗi môi trường.
4. **Báo cáo** dạng bảng: spec · trạng thái · nguyên nhân · fix tối thiểu đề xuất.
   KHÔNG tự sửa code app trong lệnh này — chỉ đề xuất; fix đi qua review riêng.
5. **Quarantine** test flaky (đánh dấu, không xoá) và ghi lý do.

## Lưu ý
- Đây là vòng QA *thường trú*: chạy sau mỗi thay đổi FE đáng kể và trên CI (job `e2e`).
- Tham chiếu thiết kế: `doc_v2/14-frontend-architecture.md`, `doc_v2/10` §3 (CI gates).
- Chrome dùng `channel: "chrome"` (xem `frontend/playwright.config.ts`).
