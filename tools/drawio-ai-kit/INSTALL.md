# Cài drawio-aws-architect cho Claude Desktop

## Yêu cầu
- macOS
- Node.js 18+ (`brew install node`)
- Claude Desktop đã cài

## Cài đặt

1. Copy thư mục `drawio-ai-kit` về máy
2. Chạy:
   ```bash
   cd drawio-ai-kit
   bash install_desktop.sh
   ```
3. **Restart Claude Desktop**

Xong. Thử ngay: *"Vẽ kiến trúc AWS 3-tier web app"*

---

Script tự động:
- Cài npm deps
- Tạo symlink skill `drawio-aws-architect`
- Đăng ký MCP server `drawio-ai-kit` vào `~/Library/Application Support/Claude/claude_desktop_config.json`
