#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/common.sh"

if [[ $# -lt 1 || $# -gt 2 ]]; then
  echo "用法：$0 <claude-code-项目目录> [agent-id]" >&2
  exit 1
fi
PROJECT_DIR="$1"
AGENT_ID="${2:-}"
MCP_FILE="$PROJECT_DIR/.mcp.json"
PYTHON="$(mcp_python)"
SERVER="$(installed_root)/mcp-server/server.py"

[[ -x "$PYTHON" ]] || { echo "未找到 MCP venv，请先执行 install.sh。" >&2; exit 1; }
[[ -d "$PROJECT_DIR" ]] || { echo "项目目录不存在：$PROJECT_DIR" >&2; exit 1; }
if [[ -z "$AGENT_ID" ]]; then
  AGENT_FILE="$PROJECT_DIR/.pingo/agent.yaml"
  [[ -f "$AGENT_FILE" ]] || { echo "未找到项目身份：$AGENT_FILE。请先在项目目录执行 pingo init。" >&2; exit 1; }
  AGENT_ID="$(python3 - "$AGENT_FILE" <<'PY'
import sys
for line in open(sys.argv[1], encoding='utf-8'):
    if line.split(':', 1)[0].strip() == 'agent_id':
        value = line.split(':', 1)[1].strip()
        if (value.startswith('"') and value.endswith('"')) or (value.startswith("'") and value.endswith("'")):
            value = value[1:-1]
        print(value)
        break
PY
)"
fi
[[ -n "$AGENT_ID" ]] || { echo "项目身份缺少 agent_id。" >&2; exit 1; }

python3 - "$MCP_FILE" "$AGENT_ID" "$PYTHON" "$SERVER" <<'PY'
import json
import pathlib
import sys

path = pathlib.Path(sys.argv[1])
agent_id, python, server = sys.argv[2:]
data = json.loads(path.read_text()) if path.exists() else {}
servers = data.setdefault("mcpServers", {})
servers[f"pingo-{agent_id}"] = {
    "command": python,
    "args": [server],
    "env": {
        "PINGO_AGENT_ID": agent_id,
        "PINGO_SIDECAR_URL": "http://127.0.0.1:19191",
    },
}
path.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n")
PY

echo "已写入 Claude Code MCP 配置：$MCP_FILE"
echo "绑定 Agent：$AGENT_ID"
echo "请重启该项目中的 Claude Code，使 MCP 配置生效。"
