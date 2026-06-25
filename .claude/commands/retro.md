---
description: Sprint retrospective — tổng kết chu kỳ dev thành postmortem có cấu trúc, lưu vào doc_v2/retros/ làm corpus seed cho RAG GĐ5.
argument-hint: "[tên sprint/feature, vd: gd2-alerting-bc]"
allowed-tools: Bash, Read, Grep, Glob, Write
---

# /retro — Nghi thức sprint + retrospective

Chạy cuối mỗi sprint/feature. Mục đích kép:
1. **Chuẩn hoá vòng dev** — bài học được ghi lại có cấu trúc thay vì trôi đi.
2. **Nuôi corpus cho GĐ5** — các file `doc_v2/retros/*.md` là dữ liệu thật để WeKnora index
   cho RAG runbook/postmortem (xem `doc_v2/17-ai-incident-automation.md`). Dogfood văn hoá
   postmortem TRƯỚC khi AI tự động hoá nó.

## Quy trình

1. Lấy bối cảnh chu kỳ: `git log --oneline <since>..HEAD`, PR đã merge, test/coverage delta.
2. Hỏi (hoặc suy ra) tên sprint từ `$ARGUMENTS`.
3. Sinh file `doc_v2/retros/YYYY-MM-DD-<slug>.md` theo template dưới.
4. Cập nhật action items có owner + due date; liên kết tới ADR nếu có quyết định mới.

## Template (ghi nguyên cấu trúc này — để RAG parse ổn định)

```markdown
# Retro — <sprint/feature> (<ngày>)

## Bối cảnh
<phạm vi, mục tiêu sprint, DoD tham chiếu 12-roadmap.md>

## Đã ship
- <thay đổi · commit/PR>

## Vỡ ở đâu / điều bất ngờ
- <sự cố, bug, hiểu lầm, lệch ước lượng — nêu nguyên nhân gốc nếu rõ>

## Quyết định đã chốt
- <quyết định · link ADR nếu có>

## Bài học (để RAG GĐ5)
- **Triệu chứng:** <...>  **Nguyên nhân:** <...>  **Cách xử lý:** <...>

## Action items
- [ ] <việc> — owner: <ai> — due: <ngày>
```

## Lưu ý
- Một file = một chu kỳ. Giữ ngắn, ưu tiên mục "Bài học" (đó là phần RAG dùng).
- KHÔNG bịa dữ liệu — chỉ ghi điều thật xảy ra trong sprint.
