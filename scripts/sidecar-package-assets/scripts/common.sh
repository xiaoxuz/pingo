#!/usr/bin/env bash
set -euo pipefail

PACKAGE_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PINGO_HOME="${PINGO_HOME:-$HOME/.pingo}"
PINGO_INSTALL_DIR="${PINGO_INSTALL_DIR:-$HOME/.local/share/pingo-sidecar}"
PINGO_CONFIG_PATH="${PINGO_CONFIG_PATH:-$PINGO_HOME/config.yaml}"
PINGO_RUNTIME_DIR="${PINGO_RUNTIME_DIR:-$PINGO_HOME/runtime}"
PINGO_PID_FILE="$PINGO_RUNTIME_DIR/sidecar.pid"
PINGO_LOG_FILE="$PINGO_RUNTIME_DIR/sidecar.log"

operation_log() {
  mkdir -p "$PINGO_RUNTIME_DIR"
  python3 - "$PINGO_LOG_FILE" "$1" "$2" "${3:-ok}" <<'PY'
import json, sys
from datetime import datetime, timezone
with open(sys.argv[1], 'a', encoding='utf-8') as output:
    output.write(json.dumps({'time': datetime.now(timezone.utc).isoformat(), 'level': 'INFO', 'msg': 'sidecar_operation', 'category': sys.argv[2], 'action': sys.argv[3], 'result': sys.argv[4]}, ensure_ascii=False) + '\n')
PY
}

installed_root() {
  if [[ -x "$PINGO_INSTALL_DIR/bin/pingo-sidecar" ]]; then
    printf '%s\n' "$PINGO_INSTALL_DIR"
  else
    printf '%s\n' "$PACKAGE_ROOT"
  fi
}

sidecar_binary() {
  printf '%s/bin/pingo-sidecar\n' "$(installed_root)"
}

mcp_python() {
  printf '%s/mcp-server/.venv/bin/python\n' "$(installed_root)"
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "缺少必需命令：$1" >&2
    exit 1
  }
}

is_running() {
  [[ -f "$PINGO_PID_FILE" ]] || return 1
  local pid
  pid="$(cat "$PINGO_PID_FILE")"
  kill -0 "$pid" 2>/dev/null
}

remove_legacy_hub_config() {
  [[ -f "$PINGO_CONFIG_PATH" ]] || return 0
  python3 - "$PINGO_CONFIG_PATH" <<'PY'
import pathlib, sys
path = pathlib.Path(sys.argv[1])
lines = path.read_text().splitlines(True)
out = []
skipping = False
changed = False
for line in lines:
    is_top_level = line and not line.startswith((' ', '\t', '\n', '\r'))
    if is_top_level and line.split(':', 1)[0] == 'hub':
        skipping = True
        changed = True
        continue
    if skipping and is_top_level:
        skipping = False
    if not skipping:
        out.append(line)
if changed:
    path.write_text(''.join(out))
PY
}
