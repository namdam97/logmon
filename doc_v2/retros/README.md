# Retros — Corpus postmortem dev

Mỗi file ở đây là một **retrospective** của một sprint/feature, sinh bởi lệnh `/retro`
(xem `.claude/commands/retro.md`).

## Vì sao tồn tại
- **Chuẩn hoá vòng dev:** bài học được ghi có cấu trúc thay vì trôi đi.
- **Seed cho RAG GĐ5:** đây là dữ liệu thật để WeKnora index cho RAG runbook/postmortem
  (`doc_v2/17-ai-incident-automation.md`). Ta dogfood văn hoá postmortem *trước* khi AI tự động hoá nó.

## Quy ước
- Tên file: `YYYY-MM-DD-<slug>.md` (1 file = 1 chu kỳ).
- Cấu trúc cố định (template trong `.claude/commands/retro.md`) để RAG parse ổn định.
- Mục **"Bài học"** (Triệu chứng / Nguyên nhân / Cách xử lý) là phần RAG dùng nhiều nhất — ưu tiên viết kỹ.
- Chỉ ghi điều thật xảy ra. KHÔNG bịa.

> Khi GĐ5 khởi động, thư mục này là một nguồn index đầu tiên cho knowledge base.
