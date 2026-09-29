#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

cat > "$TMP/release.conf" <<'CONF'
PINGO_VERSION="0.1.0"
PINGO_CHANNEL="stable"
PINGO_HUB_WS_URL="wss://hub.example.com/ws"
PINGO_UPDATE_MANIFEST_URL="https://updates.example.com/latest.json"
PINGO_UPDATE_CHECK_INTERVAL="6h"
CONF
if PINGO_RELEASE_CONFIG="$TMP/release.conf" "$ROOT/scripts/package-pingo.sh" "$TMP/invalid" >"$TMP/output" 2>&1; then
  echo '示例发布地址必须被拒绝' >&2
  exit 1
fi
sed 's/hub.example.com/pingo.test.invalid/; s/updates.example.com/updates.test.invalid/' "$TMP/release.conf" > "$TMP/real.conf"
PINGO_RELEASE_CONFIG="$TMP/real.conf" make -C "$ROOT" build-sidecar >/dev/null
PINGO_UPDATE_MANIFEST_PATH="$TMP/latest.json" PINGO_RELEASE_CONFIG="$TMP/real.conf" "$ROOT/scripts/package-pingo.sh" "$TMP/package"
! grep -q 'endpoint:' "$TMP/package/config.example.yaml"
! grep -q 'pingo.test.invalid/ws' "$TMP/package/release.json"
[[ "$("$TMP/package/bin/pingo-sidecar" --print-hub-endpoint)" == 'wss://pingo.test.invalid/ws' ]]
grep -q '"version": "0.1.0"' "$TMP/package/release.json"
test -f "$TMP/package/docs/agent/pingo.md"
test -f "$TMP/package/docs/agent/install.md"
test -f "$TMP/package/docs/agent/usage.md"
test -f "$TMP/package/docs/pingo-sidecar-operations.md"
test ! -e "$TMP/package/docs/agent.md"
test ! -e "$TMP/package/website"
grep -q 'Pingo 是你的社交能力' "$TMP/package/skills/pingo/SKILL.md"
grep -q '普通社交由你自主处理' "$TMP/package/skills/pingo/SKILL.md"
grep -q '社交过程必须在当前 Claude Code 或 Codex 窗口中进行并显示' "$TMP/package/skills/pingo/SKILL.md"
grep -q '处理成功后标记已读' "$TMP/package/skills/pingo/SKILL.md"
grep -q '只有不确定能力边界、工作方式或工具参数时才重新加载' "$TMP/package/skills/pingo/SKILL.md"
! grep -q '总结.*主人' "$TMP/package/skills/pingo/SKILL.md"
test -f "$ROOT/dist/pingo-sidecar-package-0.1.0.tar.gz"
test -f "$ROOT/dist/pingo-sidecar-package.latest.tar.gz"
grep -q 'pingo-sidecar-package.latest.tar.gz' "$TMP/latest.json"
