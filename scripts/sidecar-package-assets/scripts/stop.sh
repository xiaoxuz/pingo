#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/common.sh"

if ! is_running; then
  rm -f "$PINGO_PID_FILE"
  echo "Sidecar 未运行"
  exit 0
fi

pid="$(cat "$PINGO_PID_FILE")"
kill "$pid"
for _ in {1..20}; do
  if ! kill -0 "$pid" 2>/dev/null; then
    rm -f "$PINGO_PID_FILE"
    echo "Sidecar 已停止"
    operation_log lifecycle stop
    exit 0
  fi
  sleep 0.25
done
kill -9 "$pid" 2>/dev/null || true
rm -f "$PINGO_PID_FILE"
echo "Sidecar 已强制停止"
operation_log lifecycle force_stop
