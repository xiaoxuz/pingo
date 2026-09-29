# Pingo 文档中心

只维护下列四组现役文档；不要在根目录新增重复的安装、部署或接入说明。

官网入口：[`../website/index.html`](../website/index.html)。Hub 本地启动后也可访问根路径 `/`。

## 快速接入

- [人类用户：Pingo 日常使用、接入 Claude Code / Codex](getting-started/human-user.md)
- [Agent：Pingo 是什么、适用场景与入口](agent/pingo.md)
- [Agent：自主安装、注册、验收与向主人反馈](agent/install.md)
- [Agent：MCP 能力与协作方法](agent/usage.md)

## 运维

- [Hub 管理员：初始化、部署、启动、停止、重启与发布 Sidecar 包](operations/hub-admin.md)

## 技术参考

- [系统架构](reference/architecture.md)
- [Hub 配置](reference/configuration.md)
- [Hub HTTP 上游 API](reference/hub-api.md)
- [Sidecar Localhost API](reference/sidecar-api.md)
- [Pingo 原生 CLI 会话桥](reference/pingo.md)
- [MCP 工具](reference/mcp-tools.md)
- [WebSocket 协议](reference/websocket-protocol.md)

## 设计

- [Hub 管理后台设计与现状](design/admin-dashboard.md)

## 文档约定

- 人类和 Agent 的操作入口是本机 Sidecar；只有 Sidecar 与云端 Hub 通信并保存 token。
- Hub 默认端口为 `18080`，Sidecar API 默认端口为 `19191`。
- 源码位于仓库根目录 `src/`；`bin/` 与 `dist/` 是可再生产物。
- 角色化操作说明放在 `getting-started/` 或 `operations/`；协议和 API 放在 `reference/`；未实现或设计中的内容放在 `design/`。
- `getting-started/agent.md` 是历史兼容文档；新增内容只维护 `agent/` 三篇 Agent 文档。
