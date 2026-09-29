#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
UPDATE_SCRIPT="${1:-$ROOT/scripts/sidecar-package-assets/scripts/update.sh}"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/install/bin" "$TMP/install/scripts" "$TMP/home" "$TMP/stubs"
touch "$TMP/install/bin/pingo-sidecar"
chmod +x "$TMP/install/bin/pingo-sidecar"
cat > "$TMP/install/scripts/start.sh" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
touch "$PINGO_HOME/started"
SH
chmod +x "$TMP/install/scripts/start.sh"
cat > "$TMP/install/release.json" <<'JSON'
{"version":"0.1.1","channel":"stable","update_manifest_url":"http://127.0.0.1:18080/latest.json"}
JSON
cat > "$TMP/stubs/curl" <<'SH'
#!/usr/bin/env bash
while (( $# )); do
  if [[ "$1" == '--output' ]]; then
    shift
    output="$1"
  fi
  shift
done
printf '{"version":"0.1.1","channel":"stable","package_url":"http://127.0.0.1:18080/pingo-sidecar-package.latest.tar.gz","sha256":"%064d"}\n' 0 > "$output"
SH
chmod +x "$TMP/stubs/curl"
PINGO_INSTALL_DIR="$TMP/install" PINGO_HOME="$TMP/home" PATH="$TMP/stubs:$PATH" \
  bash "$UPDATE_SCRIPT" > "$TMP/output" 2>&1 || {
    cat "$TMP/output" >&2
    exit 1
  }
grep -q '已是最新版本：0.1.1' "$TMP/output" || {
  cat "$TMP/output" >&2
  exit 1
}
test -f "$TMP/home/started" || {
  cat "$TMP/output" >&2
  echo '已是最新版本时也必须确保 Sidecar 已启动' >&2
  exit 1
}

mkdir -p "$TMP/archive/pingo-sidecar-package/scripts" "$TMP/archive/pingo-sidecar-package/bin" "$TMP/archive/pingo-sidecar-package/mcp-server"
touch "$TMP/archive/pingo-sidecar-package/scripts/install.sh" "$TMP/archive/pingo-sidecar-package/scripts/start.sh" \
  "$TMP/archive/pingo-sidecar-package/bin/pingo-sidecar" \
  "$TMP/archive/pingo-sidecar-package/mcp-server/requirements.txt"
chmod +x "$TMP/archive/pingo-sidecar-package/scripts/start.sh" "$TMP/archive/pingo-sidecar-package/bin/"*
cat > "$TMP/archive/pingo-sidecar-package/release.json" <<'JSON'
{"version":"0.1.2","channel":"stable","update_manifest_url":"http://127.0.0.1:18080/latest.json"}
JSON
tar -czf "$TMP/stubs/package.tar.gz" -C "$TMP/archive" pingo-sidecar-package
sha="$(shasum -a 256 "$TMP/stubs/package.tar.gz" | awk '{print $1}')"
cat > "$TMP/stubs/curl" <<SH
#!/usr/bin/env bash
while (( \$# )); do
  if [[ "\$1" == '--output' ]]; then shift; output="\$1"; fi
  shift
done
if [[ "\$output" == */latest.json ]]; then
  printf '%s\\n' '{"version":"0.1.2","channel":"stable","package_url":"http://127.0.0.1:18080/pingo-sidecar-package.latest.tar.gz","sha256":"$sha"}' > "\$output"
else
  cp "$TMP/stubs/package.tar.gz" "\$output"
fi
SH
chmod +x "$TMP/stubs/curl"
PINGO_INSTALL_DIR="$TMP/install" PINGO_HOME="$TMP/home" PATH="$TMP/stubs:$PATH" \
  bash "$UPDATE_SCRIPT" > "$TMP/output" 2>&1 && {
    echo '不应安装测试包' >&2
    exit 1
  }
grep -q '更新包结构无效' "$TMP/output" || {
  cat "$TMP/output" >&2
  exit 1
}

mkdir -p "$TMP/install/mcp-server/.venv/bin" "$TMP/archive/pingo-sidecar-package/skills/pingo"
printf '#!/usr/bin/env bash\nexit 0\n' > "$TMP/install/mcp-server/.venv/bin/pip"
chmod +x "$TMP/install/mcp-server/.venv/bin/pip"
touch "$TMP/archive/pingo-sidecar-package/bin/pingo" "$TMP/archive/pingo-sidecar-package/skills/pingo/SKILL.md"
chmod +x "$TMP/archive/pingo-sidecar-package/bin/pingo"
tar -czf "$TMP/stubs/package.tar.gz" -C "$TMP/archive" pingo-sidecar-package
sha="$(shasum -a 256 "$TMP/stubs/package.tar.gz" | awk '{print $1}')"
sed "s/sha256\":\"[0-9a-f]*\"/sha256\":\"$sha\"/" "$TMP/stubs/curl" > "$TMP/stubs/curl.next"
mv "$TMP/stubs/curl.next" "$TMP/stubs/curl"
chmod +x "$TMP/stubs/curl"
PINGO_INSTALL_DIR="$TMP/install" PINGO_HOME="$TMP/home" HOME="$TMP/home" PATH="$TMP/stubs:$PATH" \
  bash "$UPDATE_SCRIPT" > "$TMP/output" 2>&1 || {
    cat "$TMP/output" >&2
    exit 1
  }
grep -q 'Pingo 更新至 0.1.2' "$TMP/output" || {
  cat "$TMP/output" >&2
  exit 1
}
