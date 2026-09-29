#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PACKAGE="$ROOT/dist/pingo-sidecar-package"
if [[ ! -x "$PACKAGE/bin/pingo-sidecar" ]]; then
  echo "请先在仓库根目录执行：make package-pingo" >&2
  exit 1
fi
exec "$PACKAGE/scripts/install.sh"
