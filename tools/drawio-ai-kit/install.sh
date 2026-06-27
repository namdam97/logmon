#!/usr/bin/env bash
# drawio-ai-kit — one-shot installer for Claude Code (MCP server + Skill).
# Idempotent: safe to re-run. Run it from anywhere; it locates the kit by this
# script's own path (so it works whether you cloned manually or piped it in).
set -euo pipefail

REPO="git@github.com:sparklabx/drawio-ai-kit.git"
SKILL_NAME="drawio-aws-architect"
MCP_NAME="drawio-ai-kit"

# --- preflight: required tools ---------------------------------------------
need() {
  command -v "$1" >/dev/null 2>&1 || { echo "✗ '$1' not found on PATH — $2"; MISSING=1; }
}
MISSING=0
need git   "install git first"
need node  "install Node.js >= 18 (e.g. 'nvm install --lts' or 'brew install node')"
need npm   "comes with Node.js — reinstall Node if missing"
need claude "install Claude Code CLI: https://docs.claude.com/claude-code"
[ "$MISSING" = 0 ] || { echo; echo "→ Fix the above and re-run."; exit 1; }

NODE_MAJOR="$(node -p 'process.versions.node.split(".")[0]' 2>/dev/null || echo 0)"
[ "$NODE_MAJOR" -ge 18 ] 2>/dev/null || echo "⚠  node $(node -v) is old; the kit needs Node 18+ (LTS recommended)."

# --- locate (or clone) the kit ---------------------------------------------
if [ -f "${BASH_SOURCE[0]:-}" ] && [ -f "$(dirname "${BASH_SOURCE[0]}")/src/mcp-server.mjs" ]; then
  KIT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"          # running from inside the clone
elif [ -f "src/mcp-server.mjs" ]; then
  KIT="$(pwd)"
else
  echo "→ Cloning $REPO ..."
  git clone "$REPO"
  KIT="$(cd drawio-ai-kit && pwd)"
fi
echo "→ Kit: $KIT"

# --- deps -------------------------------------------------------------------
echo "→ Installing npm deps ..."
( cd "$KIT" && npm install --silent )

# --- resolve absolute node (MCP is spawned with a bare env) -----------------
NODE_BIN="$(command -v node)"
[ -n "$NODE_BIN" ] || { echo "✗ node not found on PATH"; exit 1; }
echo "→ node: $NODE_BIN"

# --- 1) MCP server (re-add cleanly so re-runs don't error) ------------------
echo "→ Registering MCP server '$MCP_NAME' ..."
claude mcp remove "$MCP_NAME" --scope user >/dev/null 2>&1 || true
claude mcp remove "$MCP_NAME" >/dev/null 2>&1 || true
claude mcp add "$MCP_NAME" --scope user -- "$NODE_BIN" "$KIT/src/mcp-server.mjs"

# --- 2) Skill (symlink) -----------------------------------------------------
echo "→ Linking skill '$SKILL_NAME' ..."
mkdir -p "$HOME/.claude/skills"
ln -sfn "$KIT" "$HOME/.claude/skills/$SKILL_NAME"

# --- summary ----------------------------------------------------------------
# ponytail: no live `claude mcp list` probe here — it runs before the restart and
# often prints "✘ Failed to connect" (cold-start timeout), which looks like a failure
# but isn't. Just report what was installed and how to verify AFTER restarting.
echo
echo "✓ Installed:"
echo "  • MCP server '$MCP_NAME'  (user scope)  →  $NODE_BIN $KIT/src/mcp-server.mjs"
echo "  • Skill '$SKILL_NAME'  →  $HOME/.claude/skills/$SKILL_NAME"
echo
echo "⚠  RESTART Claude Code now — MCP servers & skills load only at session start."
echo "   After restarting, verify:   claude mcp list      (expect: $MCP_NAME ✔ Connected)"
