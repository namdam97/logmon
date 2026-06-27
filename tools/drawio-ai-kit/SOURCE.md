# Provenance — drawio-ai-kit (vendored)

- **Source:** https://github.com/sparklabx/drawio-ai-kit
- **Commit (pinned):** `bda82a2c34edc680cfb1e52e17d5b4416e6cf7c4` (2026-06-25)
- **Vendored:** 2026-06-26
- **License:** MIT (xem `LICENSE`, `NOTICE`, `THIRD_PARTY_NOTICES.md` — icon là trademark của chủ sở hữu, dùng để nhận diện như cách draw.io ship icon AWS/Azure)
- **Vendor lý do:** kiểm soát qua git + review được + áp bản vá bảo mật, thay vì `install.sh` (clone động + `npm install` floating + cài user-scope global).

## Tích hợp với LogMon

- Đăng ký **MCP project-scoped** qua `../../.mcp.json` (chỉ load cho repo này). Tools: `search_icon`, `get_icon_style`, `validate_diagram`, `get_principles`, `render_diagram`, `brand_logo`.
- Cài deps: `cd tools/drawio-ai-kit && npm ci --ignore-scripts --omit=dev` (đã pin `package-lock.json`; `node_modules/` bị gitignore — chạy lệnh này sau khi clone repo).
- `render_diagram` cần **draw.io desktop CLI** (`drawio`) trên PATH hoặc set `DRAWIO_CLI`. Trên Linux headless cần `xvfb-run` + có thể cần `DRAWIO_NO_SANDBOX=1` (xem bản vá R2). Thiếu CLI thì `render_diagram` trả lỗi êm, `search/validate` vẫn chạy bình thường.
- Phù hợp để sinh/lint sơ đồ kiến trúc trong `doc_v2/` — catalog có sẵn icon Kafka / Kubernetes / Prometheus / Grafana / Postgres / Redis / OpenTelemetry… đúng stack LogMon.

## Bản vá bảo mật đã áp (khác với upstream)

Sau khi audit (security-reviewer + đọc tay toàn bộ runtime), đã sửa `src/mcp-server.mjs`:

- **R1 (HIGH) — path traversal:** `render_diagram` nay validate `args.path` (ép đuôi `.drawio`/`.xml` + `existsSync` + `resolve`) trước khi đưa vào draw.io CLI → chặn prompt-injection lừa render file nhạy cảm (`~/.ssh/id_rsa`, `.env`).
- **R2 (MEDIUM) — Chromium sandbox:** `--no-sandbox` chuyển thành **opt-in** qua env `DRAWIO_NO_SANDBOX=1` (mặc định BẬT sandbox). Chỉ tắt khi render trên Linux/CI headless.

> Khi bump version upstream: re-clone commit mới, **áp lại R1/R2**, chạy `npm ci` + `npm audit`, cập nhật commit pin ở trên.

## Kết luận audit (2026-06-26): GO WITH CONDITIONS → đã thoả

- ✅ Không lifecycle hook (`package.json`/lock 0 `hasInstallScript`); `npm ci` → **0 vulnerabilities**; dep tree mainstream (MCP SDK 1.29.0 → express/hono/ajv/zod/jose…).
- ✅ Mọi shell-out dùng `execFileSync` + mảng args (không qua shell → không command injection). Không `eval`/`new Function`.
- ✅ Không SSRF (`brand_logo`/`query` chỉ search trên manifest cố định; URL fetch giới hạn `unpkg.com` + `cdn.simpleicons.org`). Không XXE (validate parse XML bằng regex tuyến tính, không entity expansion).
- ✅ Không secret, không commit `node_modules`, CI pin action + `permissions: contents:read` + `npm audit`.
- ⚠️ R1/R2 đã vá ở trên. LOW còn lại (chỉ ghi nhận): `DRAWIO_CATALOG` env đọc JSON tuỳ ý lúc start (env do user kiểm soát); `--embed` của `brand_logo` inline SVG từ CDN (phụ thuộc độ tin CDN — tránh bằng cách không dùng `embed:true`).

## Scripts maintainer-only (KHÔNG chạy lúc install/runtime)

`scripts/crawl_icons.py`, `scripts/build_pack.py`, `scripts/ingest_index.py`, `vendor/autolayout.py`, `vendor/encode_drawio_url.py`, `vendor/repair_png.py` — chỉ chạy thủ công qua `npm run gen:catalog` để regenerate catalog (có network fetch + subprocess, args dạng list, không injection). Không nằm trong đường chạy của MCP server.
