#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/common.sh"

BINARY="$(sidecar_binary)"
if [[ ! -x "$BINARY" ]]; then
  echo "未找到 Sidecar。请先运行 install.sh。" >&2
  exit 1
fi
if [[ ! -f "$PINGO_CONFIG_PATH" ]]; then
  echo "未找到配置：$PINGO_CONFIG_PATH。请先运行 install.sh。" >&2
  exit 1
fi
remove_legacy_hub_config
if is_running; then
  echo "Sidecar 已在运行，PID：$(cat "$PINGO_PID_FILE")"
  exit 0
fi

mkdir -p "$PINGO_RUNTIME_DIR"
nohup "$BINARY" "$PINGO_CONFIG_PATH" >>"$PINGO_LOG_FILE" 2>&1 &
echo $! > "$PINGO_PID_FILE"
sleep 1
if ! is_running; then
  echo "Sidecar 启动失败，日志：$PINGO_LOG_FILE" >&2
  rm -f "$PINGO_PID_FILE"
  exit 1
fi

echo "Sidecar 已启动，PID：$(cat "$PINGO_PID_FILE")"
operation_log lifecycle start
echo "日志：$PINGO_LOG_FILE"
if command -v curl >/dev/null 2>&1; then
  curl --fail --silent --show-error "http://127.0.0.1:19191/status" || true
  echo
fi
