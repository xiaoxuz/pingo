#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/common.sh"

PURGE=0
if [[ "${1:-}" == "--purge" ]]; then
  PURGE=1
elif [[ $# -gt 0 ]]; then
  echo "用法：$0 [--purge]" >&2
  echo "默认只删除安装目录，保留 ~/.pingo 中的身份、配置、SQLite 和日志。" >&2
  exit 1
fi

if is_running; then
  "$(installed_root)/scripts/stop.sh"
fi

skill_source="$PACKAGE_ROOT/skills/pingo/SKILL.md"
skill_matches=()
for skill_path in "$HOME/.claude/skills/pingo/SKILL.md" "$HOME/.codex/skills/pingo/SKILL.md"; do
  if [[ -f "$skill_path" && -f "$skill_source" ]] && cmp -s "$skill_path" "$skill_source"; then
    skill_matches+=("$skill_path")
  fi
done

if [[ -d "$PINGO_INSTALL_DIR" ]]; then
  rm -rf "$PINGO_INSTALL_DIR"
  echo "已删除 Pingo 安装目录：$PINGO_INSTALL_DIR"
else
  echo "未找到 Pingo 安装目录：$PINGO_INSTALL_DIR"
fi

for skill_path in "${skill_matches[@]}"; do
  rm -f "$skill_path"
  skill_dir="$(dirname "$skill_path")"
  rmdir "$skill_dir" 2>/dev/null || true
done

if (( PURGE )); then
  rm -rf "$PINGO_HOME"
  echo "已彻底删除 Pingo 本机数据：$PINGO_HOME"
else
  mkdir -p "$PINGO_RUNTIME_DIR"
  operation_log lifecycle uninstall
  echo "已保留本机数据：$PINGO_HOME"
  echo "如需彻底删除身份、token、本地数据库和日志，执行：$0 --purge"
fi
