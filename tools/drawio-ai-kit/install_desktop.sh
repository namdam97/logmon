#!/usr/bin/env bash
# drawio-ai-kit — installer for Claude Desktop (macOS).
# Idempotent: safe to re-run.
set -euo pipefail

SKILL_NAME="drawio-aws-architect"
MCP_NAME="drawio-ai-kit"
CLAUDE_CONFIG="$HOME/Library/Application Support/Claude/claude_desktop_config.json"

# --- locate kit (script's own directory) ------------------------------------
KIT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
echo "→ Kit: $KIT"

# --- preflight --------------------------------------------------------------
need() {
  command -v "$1" >/dev/null 2>&1 || { echo "✗ '$1' not found — $2"; MISSING=1; }
}
MISSING=0
need node  "install Node.js >= 18: brew install node"
need npm   "comes with Node.js"
[ "$MISSING" = 0 ] || { echo; echo "→ Fix the above and re-run."; exit 1; }

NODE_MAJOR="$(node -p 'process.versions.node.split(".")[0]' 2>/dev/null || echo 0)"
[ "$NODE_MAJOR" -ge 18 ] 2>/dev/null || echo "⚠  node $(node -v) is too old; need Node 18+."

NODE_BIN="$(command -v node)"

# --- 1) npm deps -------------------------------------------------------------
echo "→ Installing npm deps ..."
( cd "$KIT" && npm install --silent )

# --- 2) Skill symlink --------------------------------------------------------
echo "→ Linking skill '$SKILL_NAME' ..."
mkdir -p "$HOME/.claude/skills"
ln -sfn "$KIT" "$HOME/.claude/skills/$SKILL_NAME"

# --- 3) Claude Desktop MCP config -------------------------------------------
echo "→ Registering MCP server in Claude Desktop config ..."

mkdir -p "$HOME/Library/Application Support/Claude"

# Đọc config hiện tại (hoặc tạo mới)
if [ -f "$CLAUDE_CONFIG" ]; then
  CURRENT="$(cat "$CLAUDE_CONFIG")"
else
  CURRENT="{}"
fi

# Dùng Python (có sẵn trên macOS) để merge JSON
python3 - "$CLAUDE_CONFIG" "$NODE_BIN" "$KIT" "$MCP_NAME" <<'PYEOF'
import json, sys

config_path = sys.argv[1]
node_bin    = sys.argv[2]
kit_path    = sys.argv[3]
mcp_name    = sys.argv[4]

try:
    with open(config_path) as f:
        cfg = json.load(f)
except (FileNotFoundError, json.JSONDecodeError):
    cfg = {}

cfg.setdefault("mcpServers", {})[mcp_name] = {
    "command": node_bin,
    "args": [f"{kit_path}/src/mcp-server.mjs"]
}

with open(config_path, "w") as f:
    json.dump(cfg, f, indent=2)
    f.write("\n")

print(f"  Updated: {config_path}")
PYEOF

# --- summary -----------------------------------------------------------------
echo
echo "✓ Installed:"
echo "  • Skill '$SKILL_NAME'     → $HOME/.claude/skills/$SKILL_NAME"
echo "  • MCP server '$MCP_NAME'  → $NODE_BIN $KIT/src/mcp-server.mjs"
echo "  • Config: $CLAUDE_CONFIG"
echo
echo "⚠  Restart Claude Desktop — MCP servers load at startup."
echo "   Sau khi restart, thử: 'Vẽ kiến trúc AWS 3-tier web app'"
