#!/usr/bin/env bash
set -euo pipefail
if [[ -z "${PINGO_UPDATE_SNAPSHOT:-}" ]]; then
  snapshot="$(mktemp -d)"
  cp "$0" "$(dirname "$0")/common.sh" "$snapshot/"
  if PINGO_UPDATE_SNAPSHOT=1 bash "$snapshot/update.sh" "$@"; then
    result=0
  else
    result=$?
  fi
  rm -r "$snapshot"
  exit "$result"
fi
source "$(dirname "$0")/common.sh"

require_command python3
require_command curl
require_command tar
curl_protocol_flags() {
  case "$1" in
    https://*) printf '%s\n' "--proto =https --proto-redir =https" ;;
    http://127.0.0.1:*|http://localhost:*|http://10.*|http://192.168.*|http://172.1[6-9].*|http://172.2[0-9].*|http://172.3[0-1].*) printf '%s\n' "--proto =http,https --proto-redir =http,https" ;;
    *) echo "不允许的非 HTTPS 地址：$1" >&2; exit 1 ;;
  esac
}
[[ -x "$PINGO_INSTALL_DIR/bin/pingo-sidecar" && -f "$PINGO_INSTALL_DIR/release.json" ]] || {
  echo "请先安装含 release.json 的 Pingo 安装包" >&2; exit 1;
}

TMP_DIR="$(mktemp -d "$(dirname "$PINGO_INSTALL_DIR")/.pingo-update.XXXXXX")"
BACKUP="$TMP_DIR/backup"
switched=0
was_running=0
cleanup() {
  result=$?
  trap - EXIT
  if (( result != 0 )); then
    if (( switched )); then
      rm -rf "$PINGO_INSTALL_DIR"
      mv "$BACKUP" "$PINGO_INSTALL_DIR"
      if (( was_running )); then "$PINGO_INSTALL_DIR/scripts/start.sh" || true; fi
    fi
    operation_log update failed failed
  fi
  rm -rf "$TMP_DIR"
  exit "$result"
}
trap cleanup EXIT

python3 - "$PINGO_INSTALL_DIR/release.json" > "$TMP_DIR/release-info" <<'PY'
import json, sys
from urllib.parse import urlsplit
data = json.load(open(sys.argv[1]))
url = data['update_manifest_url']
parts = urlsplit(url)
host = parts.hostname or ''
private_http = parts.scheme == 'http' and (host == 'localhost' or host == '127.0.0.1' or host.startswith('10.') or host.startswith('192.168.') or any(host.startswith(f'172.{i}.') for i in range(16, 32)))
if not ((parts.scheme == 'https' and parts.netloc) or (private_http and parts.netloc)):
    sys.exit('更新清单地址必须是 HTTPS，或本机/私网 HTTP')
print(url)
print(data['version'])
print(data['channel'])
PY
manifest_url="$(sed -n '1p' "$TMP_DIR/release-info")"
current_version="$(sed -n '2p' "$TMP_DIR/release-info")"
channel="$(sed -n '3p' "$TMP_DIR/release-info")"
[[ -n "$manifest_url" && -n "$current_version" && -n "$channel" ]] || { echo '无效的 release.json' >&2; exit 1; }
operation_log update check
curl --fail --location $(curl_protocol_flags "$manifest_url") --silent --show-error "$manifest_url" --output "$TMP_DIR/latest.json"
python3 - "$TMP_DIR/latest.json" "$current_version" "$channel" > "$TMP_DIR/target-info" <<'PY'
import json, re, sys
from urllib.parse import urlsplit
data = json.load(open(sys.argv[1]))
version = data['version']
current = sys.argv[2]
if not re.fullmatch(r'\d+\.\d+\.\d+', version) or not re.fullmatch(r'\d+\.\d+\.\d+', current):
    sys.exit('版本格式无效')
if data['channel'] != sys.argv[3]:
    sys.exit('更新渠道不匹配')
url = data['package_url']
parts = urlsplit(url)
host = parts.hostname or ''
private_http = parts.scheme == 'http' and (host == 'localhost' or host == '127.0.0.1' or host.startswith('10.') or host.startswith('192.168.') or any(host.startswith(f'172.{i}.') for i in range(16, 32)))
if not ((parts.scheme == 'https' and parts.netloc) or (private_http and parts.netloc)):
    sys.exit('安装包地址必须是 HTTPS，或本机/私网 HTTP')
if not re.fullmatch(r'[0-9a-fA-F]{64}', data['sha256']):
    sys.exit('SHA256 格式无效')
print(url)
print(data['sha256'].lower())
print('yes' if tuple(map(int, version.split('.'))) > tuple(map(int, current.split('.'))) else 'no')
print(version)
PY
package_url="$(sed -n '1p' "$TMP_DIR/target-info")"
expected_sha="$(sed -n '2p' "$TMP_DIR/target-info")"
update_available="$(sed -n '3p' "$TMP_DIR/target-info")"
target_version="$(sed -n '4p' "$TMP_DIR/target-info")"
[[ -n "$package_url" && -n "$expected_sha" && -n "$update_available" && -n "$target_version" ]] || { echo '更新清单无效' >&2; exit 1; }
if [[ "$update_available" != yes ]]; then
  echo "已是最新版本：$current_version"
  if ! is_running; then "$PINGO_INSTALL_DIR/scripts/start.sh"; fi
  exit 0
fi

ARCHIVE="$TMP_DIR/package.tar.gz"
curl --fail --location $(curl_protocol_flags "$package_url") --silent --show-error "$package_url" --output "$ARCHIVE"
python3 - "$ARCHIVE" "$expected_sha" "$TMP_DIR/unpacked" <<'PY'
import hashlib, pathlib, sys, tarfile
archive, expected, destination = sys.argv[1:]
digest = hashlib.sha256(pathlib.Path(archive).read_bytes()).hexdigest()
if digest != expected:
    sys.exit('SHA256 校验失败')
root = pathlib.Path(destination)
root.mkdir()
with tarfile.open(archive, 'r:gz') as tar:
    for member in tar.getmembers():
        path = pathlib.PurePosixPath(member.name)
        if path.is_absolute() or '..' in path.parts or not (member.isfile() or member.isdir()):
            sys.exit('更新包包含不安全路径或链接')
    tar.extractall(root)
PY
NEW_PACKAGE="$(find "$TMP_DIR/unpacked" -maxdepth 3 -type f -path '*/scripts/install.sh' -print -quit)"
[[ -n "$NEW_PACKAGE" ]] || { echo '更新包结构无效' >&2; exit 1; }
NEW_PACKAGE="$(cd "$(dirname "$NEW_PACKAGE")/.." && pwd)"
[[ -x "$NEW_PACKAGE/bin/pingo-sidecar" && -x "$NEW_PACKAGE/bin/pingo" && -f "$NEW_PACKAGE/release.json" && -f "$NEW_PACKAGE/mcp-server/requirements.txt" && -x "$NEW_PACKAGE/scripts/start.sh" ]] || {
  echo '更新包结构无效' >&2; exit 1;
}
python3 - "$NEW_PACKAGE/release.json" "$target_version" "$channel" "$manifest_url" <<'PY'
import json, sys
data = json.load(open(sys.argv[1]))
if (data.get('version'), data.get('channel'), data.get('update_manifest_url')) != tuple(sys.argv[2:]):
    sys.exit('更新包 release.json 与清单不一致')
PY

if is_running; then was_running=1; "$PINGO_INSTALL_DIR/scripts/stop.sh"; fi
mv "$PINGO_INSTALL_DIR" "$BACKUP"
switched=1
mv "$NEW_PACKAGE" "$PINGO_INSTALL_DIR"
mkdir -p "$HOME/.claude/skills/pingo" "$HOME/.codex/skills/pingo"
cp "$PINGO_INSTALL_DIR/skills/pingo/SKILL.md" "$HOME/.claude/skills/pingo/SKILL.md"
cp "$PINGO_INSTALL_DIR/skills/pingo/SKILL.md" "$HOME/.codex/skills/pingo/SKILL.md"
if [[ -d "$BACKUP/mcp-server/.venv" ]]; then
  cp -R "$BACKUP/mcp-server/.venv" "$PINGO_INSTALL_DIR/mcp-server/.venv"
else
  python3 -m venv "$PINGO_INSTALL_DIR/mcp-server/.venv"
fi
"$PINGO_INSTALL_DIR/mcp-server/.venv/bin/pip" install --disable-pip-version-check -r "$PINGO_INSTALL_DIR/mcp-server/requirements.txt"
"$PINGO_INSTALL_DIR/scripts/start.sh"
switched=0
operation_log update install
echo "Pingo 更新至 ${target_version}；身份与本地数据保留。"
