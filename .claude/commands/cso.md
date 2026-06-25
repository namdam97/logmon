---
description: Checklist bảo mật định kỳ (Chief Security Officer) — lái ecc:security-reviewer + tổng hợp gates, theo OWASP & doc_v2/09.
argument-hint: "[phạm vi, vd: auth | api | deps | secrets] (bỏ trống = full sweep)"
allowed-tools: Bash, Read, Grep, Glob, Task
---

# /cso — Security checklist định kỳ

Nghi thức bảo mật chạy **định kỳ** (hàng quý theo `doc_v2/10` §6) và trước release lớn.
Lõi dùng agent `ecc:security-reviewer`. Đối chiếu `doc_v2/09-security.md` + OWASP.

## Checklist (CLAUDE.md §Security + ADR-022/023/044..048)

### Tự động (đã/đang là CI gate)
- [ ] `govulncheck ./...` sạch (CI: job backend).
- [ ] `gitleaks` không phát hiện secret (CI: job secrets-scan).
- [ ] `golangci-lint` (gồm gosec khi bật — ADR-046) không lỗi bảo mật.
- [ ] Trivy fail CRIT/HIGH trên image (ADR-044) — xác nhận đang chạy.

### Rà tay (giao `ecc:security-reviewer` theo `$ARGUMENTS`)
- [ ] Không hardcode secret; secrets qua env/SOPS/Docker secrets (ADR-047).
- [ ] Input validation `validator/v10` ở mọi request struct.
- [ ] Query tham số hoá `$1,$2` (pgx) — KHÔNG nối chuỗi.
- [ ] Auth: argon2id (ADR-022), JWT HttpOnly+Secure+SameSite + refresh rotation (ADR-023).
- [ ] HTTP security headers (HSTS, X-Content-Type-Options, X-Frame-Options).
- [ ] TLS MinVersion 1.2, `InsecureSkipVerify: false`.
- [ ] `crypto/rand` cho token; lỗi trả user generic, log chi tiết nội bộ.
- [ ] Telemetry-as-untrusted (GĐ5): không để log/metric độc dẫn prompt injection (doc 17).

## Đầu ra
- Bảng: hạng mục · trạng thái (✅/⚠️/❌) · severity (CRIT/HIGH/MED/LOW) · hành động.
- CRIT → **dừng**, fix trước theo Security Response Protocol; rotate secret nếu lộ.
- Ghi kết quả định kỳ vào một retro (`/retro`) nếu phát hiện đáng học.
