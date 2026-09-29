#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/common.sh"

require_command python3
ROOT="$PACKAGE_ROOT"
BINARY="$ROOT/bin/pingo-sidecar"
if [[ ! -x "$BINARY" ]]; then
  echo "安装包缺少可执行文件：$BINARY。请使用 make package-pingo 生成安装包。" >&2
  exit 1
fi

mkdir -p "$PINGO_HOME" "$PINGO_RUNTIME_DIR" "$(dirname "$PINGO_INSTALL_DIR")"
chmod 700 "$PINGO_HOME"

install_skill() {
  local source="$1"
  local target="$2"
  mkdir -p "$(dirname "$target")"
  cp "$source" "$target"
  chmod 600 "$target"
}

if [[ -x "$PINGO_INSTALL_DIR/bin/pingo-sidecar" ]]; then
  echo "检测到已安装的 Pingo Sidecar：$PINGO_INSTALL_DIR"
  echo "复用现有安装，不覆盖正在服务其他 Agent 的 Sidecar。"
  echo "如需升级，请执行 $PINGO_INSTALL_DIR/scripts/update.sh。"
else
  mkdir -p "$PINGO_INSTALL_DIR"
  cp -R "$ROOT/bin" "$ROOT/mcp-server" "$ROOT/scripts" "$ROOT/docs" "$ROOT/skills" "$ROOT/config.example.yaml" "$ROOT/release.json" "$ROOT/README.md" "$PINGO_INSTALL_DIR/"
  chmod +x "$PINGO_INSTALL_DIR/bin/pingo-sidecar" "$PINGO_INSTALL_DIR/bin/pingo" "$PINGO_INSTALL_DIR/scripts/"*.sh
  echo "已安装 Pingo Sidecar：$PINGO_INSTALL_DIR"
fi

if [[ ! -f "$PINGO_CONFIG_PATH" ]]; then
  cp "$PINGO_INSTALL_DIR/config.example.yaml" "$PINGO_CONFIG_PATH"
  chmod 600 "$PINGO_CONFIG_PATH"
  echo "已创建配置：$PINGO_CONFIG_PATH"
fi
remove_legacy_hub_config
install_skill "$ROOT/skills/pingo/SKILL.md" "$HOME/.claude/skills/pingo/SKILL.md"
install_skill "$ROOT/skills/pingo/SKILL.md" "$HOME/.codex/skills/pingo/SKILL.md"

MCP_DIR="$PINGO_INSTALL_DIR/mcp-server"
if [[ ! -x "$MCP_DIR/.venv/bin/python" ]]; then
  python3 -m venv "$MCP_DIR/.venv"
fi
"$MCP_DIR/.venv/bin/pip" install --disable-pip-version-check -r "$MCP_DIR/requirements.txt"
operation_log lifecycle install

echo "Pingo Sidecar 安装完成"
echo "安装目录：$PINGO_INSTALL_DIR"
echo "配置文件：$PINGO_CONFIG_PATH"
echo "下一步：编辑配置后执行 $PINGO_INSTALL_DIR/scripts/start.sh"
