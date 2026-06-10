# Provenance — taste-skill

- **Source:** https://github.com/leonxlnx/taste-skill (path `skills/taste-skill/SKILL.md`)
- **Commit:** `1a6dc0a5ac5d0152120938bf66ed67ea2ec8e552`
- **Vendored:** 2026-06-11
- **License:** MIT (xem `LICENSE`)
- **Vendor lý do:** kiểm soát qua git + review được, thay vì `npx skills add` kéo nội dung động.

## Phạm vi (QUAN TRỌNG)

Theo chính frontmatter của skill: dùng cho **landing pages, portfolios, redesigns** —
**KHÔNG** dành cho *dashboards, data tables, multi-step product UI*.

→ Với LogMon: hữu ích khi làm **trang marketing / public-facing / landing**.
Với **admin dashboard data-table** (phần chính của LogMon) hãy ưu tiên
`ecc:dashboard-builder` + `ecc:design-system` + shadcn/ui, và agent
`ecc:react-reviewer` / `ecc:a11y-architect` cho phần đúng/accessible.

## An toàn

Đã scan: nội dung là design guidance, không có RCE/exfil/secret-harvest. Các lệnh
`npm install`/`npx shadcn` trong skill là khuyến nghị cài design-system chính thức
(agent đề xuất, người dùng duyệt qua permission).
