# Sơ đồ kiến trúc LogMon (drawio-ai-kit)

Các sơ đồ ở đây được sinh **bằng code** (declarative, không hardcode toạ độ) qua
[`tools/drawio-ai-kit`](../../tools/drawio-ai-kit) — layout engine + icon catalog + validator.
Source of truth nội dung: [`CLAUDE.md`](../../CLAUDE.md) và `doc_v2/`.

| Sơ đồ | File nguồn (build) | `.drawio` | `.png` |
|------|--------------------|-----------|--------|
| **Logs pipeline** — `zerolog → OTel Collector (agent→gateway) → Elasticsearch (data streams + ILM)`; Kafka chỉ ở Mode B (ADR-018) | [`examples/build_logmon_logs_pipeline.mjs`](../../tools/drawio-ai-kit/examples/build_logmon_logs_pipeline.mjs) | [logmon_logs_pipeline.drawio](logmon_logs_pipeline.drawio) | ![logs](logmon_logs_pipeline.png) |
| **Bounded Contexts map** — 6 BC + Shared Kernel + lớp AI GĐ5; cross-BC qua **domain events**, no cross-BC import | [`examples/build_logmon_bc_map.mjs`](../../tools/drawio-ai-kit/examples/build_logmon_bc_map.mjs) | [logmon_bc_map.drawio](logmon_bc_map.drawio) | ![bc](logmon_bc_map.png) |

> `.drawio` mở/sửa được trong [draw.io](https://app.diagrams.net) hoặc extension VS Code "Draw.io Integration".
> Icon thật lấy từ catalog của kit (OTel, Kafka, Elasticsearch, Kibana). Logo Grafana nhúng từ
> SVG chính chủ (catalog/lobe-icons không có) — xem `examples/_grafana_icon.mjs`.

## Regenerate (.drawio + validate)

```bash
cd tools/drawio-ai-kit
OUT=../../doc_v2/diagrams/logmon_logs_pipeline.drawio node examples/build_logmon_logs_pipeline.mjs
OUT=../../doc_v2/diagrams/logmon_bc_map.drawio        node examples/build_logmon_bc_map.mjs
```

Mỗi script tự gọi `d.validate()` (kiểm icon tồn tại, edge hợp lệ, lint layout) và phải in
`{ ok: true, errors: [], warnings: [], advice: [] }` trước khi commit.

## Render PNG (draw.io desktop CLI — headless, không cần root)

Máy CI/headless không có sẵn draw.io CLI + không có xvfb/sudo. Cách dựng userspace:

```bash
# 1. tải .deb desktop mới nhất rồi giải nén vào userspace (KHÔNG cài, không cần root)
curl -L -o /tmp/drawio.deb \
  "$(curl -s https://api.github.com/repos/jgraph/drawio-desktop/releases/latest \
     | jq -r '.assets[].browser_download_url' | grep -E 'amd64-.*\.deb$')"
dpkg-deb -x /tmp/drawio.deb ~/.local/share/drawio-extract
cp -a ~/.local/share/drawio-extract/opt/drawio ~/.local/share/drawio-desktop

# 2. wrapper trên PATH — tự chèn cờ headless (--no-sandbox --disable-gpu, KHÔNG ozone)
cat > ~/.local/bin/drawio <<'EOF'
#!/bin/sh
exec "$HOME/.local/share/drawio-desktop/drawio" --no-sandbox --disable-gpu "$@"
EOF
chmod +x ~/.local/bin/drawio

# 3. render
cd doc_v2/diagrams
drawio -x -f png -s 2 -o logmon_logs_pipeline.png logmon_logs_pipeline.drawio
drawio -x -f png -s 2 -o logmon_bc_map.png        logmon_bc_map.drawio
```

MCP tool `render_diagram` của kit tự dò `drawio` trên `PATH` → wrapper ở `~/.local/bin/drawio`
là đủ để render qua MCP (không cần restart server). Cảnh báo `vaInitialize ... libva` là warning
GPU vô hại; **tránh** `--ozone-platform=headless` (segfault trên bản 30.x).
