#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUTPUT_DIR="${1:-$ROOT/dist/pingo-sidecar-package}"
ASSETS_DIR="$ROOT/scripts/sidecar-package-assets"
BINARY="$ROOT/bin/pingo-sidecar"
PINGO_BINARY="$ROOT/bin/pingo"
source "${PINGO_RELEASE_CONFIG:-$ROOT/release/pingo-package.conf}"
HUB_WS_URL="$PINGO_HUB_WS_URL"
VERSION_DIR="$ROOT/dist/pingo-sidecar-package-$PINGO_VERSION"

if [[ ! "$HUB_WS_URL" =~ ^wss?://[^/]+/ws$ || "$HUB_WS_URL" == *'<'* || "$HUB_WS_URL" == *'hub.example.com'* ]]; then
  echo '请在 release/pingo-package.conf 填写真实 Hub 地址。' >&2
  exit 1
fi
if [[ ! "$PINGO_UPDATE_MANIFEST_URL" =~ ^https://[^/]+/ && ! "$PINGO_UPDATE_MANIFEST_URL" =~ ^http://(127\.0\.0\.1|localhost|10\.|192\.168\.|172\.(1[6-9]|2[0-9]|3[0-1])\.)[^/]+/ || "$PINGO_UPDATE_MANIFEST_URL" == *'example.com'* || ! "$PINGO_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo '请在 release/pingo-package.conf 填写版本与真实更新清单 URL。' >&2
  exit 1
fi

if [[ ! -x "$BINARY" ]]; then
  echo "未找到 Sidecar 二进制：$BINARY" >&2
  echo "请先执行 make build-sidecar，或直接执行 make package-pingo。" >&2
  exit 1
fi
if [[ ! -x "$PINGO_BINARY" ]]; then
  echo "未找到 Pingo 二进制：$PINGO_BINARY" >&2
  echo "请先执行 make build-pingo，或直接执行 make package-pingo。" >&2
  exit 1
fi
BUILTIN_HUB_WS_URL="$($BINARY --print-hub-endpoint 2>/dev/null | tail -n 1)"
if [[ "$BUILTIN_HUB_WS_URL" != "$HUB_WS_URL" ]]; then
  echo 'Sidecar 二进制内置地址不匹配，请执行 make package-pingo 重新构建。' >&2
  echo "当前二进制内置：$BUILTIN_HUB_WS_URL" >&2
  exit 1
fi

rm -rf "$OUTPUT_DIR" "$VERSION_DIR"
mkdir -p "$OUTPUT_DIR/bin" "$OUTPUT_DIR/mcp-server"

cp "$BINARY" "$OUTPUT_DIR/bin/pingo-sidecar"
cp "$PINGO_BINARY" "$OUTPUT_DIR/bin/pingo"
cp "$ROOT/src/mcp-server/server.py" "$ROOT/src/mcp-server/config.py" \
  "$ROOT/src/mcp-server/sidecar_client.py" "$ROOT/src/mcp-server/requirements.txt" \
  "$OUTPUT_DIR/mcp-server/"
cp -R "$ROOT/src/mcp-server/tools" "$OUTPUT_DIR/mcp-server/tools"
find "$OUTPUT_DIR/mcp-server" -type d -name __pycache__ -prune -exec rm -rf {} +
cp -R "$ASSETS_DIR/scripts" "$OUTPUT_DIR/"
mkdir -p "$OUTPUT_DIR/docs/agent"
cp "$ASSETS_DIR/docs/pingo-sidecar-operations.md" "$OUTPUT_DIR/docs/"
cp "$ROOT/docs/agent/pingo.md" "$ROOT/docs/agent/install.md" "$ROOT/docs/agent/usage.md" "$OUTPUT_DIR/docs/agent/"
mkdir -p "$OUTPUT_DIR/skills/pingo"
cp "$ROOT/skills/pingo/SKILL.md" "$OUTPUT_DIR/skills/pingo/"
cp "$ASSETS_DIR/README.md" "$ASSETS_DIR/config.example.yaml" \
  "$OUTPUT_DIR/"
python3 - "$OUTPUT_DIR/release.json" "$PINGO_VERSION" "$PINGO_CHANNEL" "$PINGO_UPDATE_MANIFEST_URL" "$PINGO_UPDATE_CHECK_INTERVAL" <<'PY'
import json
import sys
from pathlib import Path
Path(sys.argv[1]).write_text(json.dumps(dict(zip(('version', 'channel', 'update_manifest_url', 'update_check_interval'), sys.argv[2:])), indent=2) + '\n')
PY
chmod +x "$OUTPUT_DIR/bin/pingo-sidecar" "$OUTPUT_DIR/bin/pingo" "$OUTPUT_DIR/scripts/"*.sh

VERSION_ARCHIVE="$ROOT/dist/pingo-sidecar-package-$PINGO_VERSION.tar.gz"
LATEST_ARCHIVE="$ROOT/dist/pingo-sidecar-package.latest.tar.gz"
mkdir -p "$(dirname "$VERSION_DIR")"
rm -f "$VERSION_ARCHIVE" "$LATEST_ARCHIVE"
cp -R "$OUTPUT_DIR" "$VERSION_DIR"
COPYFILE_DISABLE=1 tar -C "$(dirname "$VERSION_DIR")" -czf "$VERSION_ARCHIVE" "$(basename "$VERSION_DIR")"
COPYFILE_DISABLE=1 tar -C "$(dirname "$OUTPUT_DIR")" -czf "$LATEST_ARCHIVE" "$(basename "$OUTPUT_DIR")"

HUB_HTTP_BASE="$(printf '%s' "$HUB_WS_URL" | sed -e 's#^wss://#https://#' -e 's#^ws://#http://#' -e 's#/ws$##')"
MANIFEST_PATH="${PINGO_UPDATE_MANIFEST_PATH:-$ROOT/latest.json}"
SHA256="$(shasum -a 256 "$LATEST_ARCHIVE" | awk '{print $1}')"
python3 - "$MANIFEST_PATH" "$PINGO_VERSION" "$PINGO_CHANNEL" "$HUB_HTTP_BASE/pingo-sidecar-package.latest.tar.gz" "$SHA256" <<'PY'
import json
import sys
from pathlib import Path
path = Path(sys.argv[1])
path.write_text(json.dumps({
    "version": sys.argv[2],
    "channel": sys.argv[3],
    "package_url": sys.argv[4],
    "sha256": sys.argv[5],
    "notes": "",
}, ensure_ascii=False, indent=2) + "\n")
PY

echo "Pingo Sidecar 安装包已生成：$OUTPUT_DIR"
echo "版本包已生成：$VERSION_ARCHIVE"
echo "最新包已生成：$LATEST_ARCHIVE"
echo "更新清单已更新：$MANIFEST_PATH"
