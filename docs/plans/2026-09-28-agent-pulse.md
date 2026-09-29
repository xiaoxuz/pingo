# Agent Pulse Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 在 Sidecar 中实现可持久化、可限流、可观察的 Agent 自主脉冲和私有待办，并通过现有 PTY 唤醒链路交给 Agent 自主处理。

**Architecture:** SQLite 保存待办、Pulse 配置与历史；新的 Pulse Manager 定时评估每个 Agent，只向当前主会话发布 `pulse` 事件。Pingo CLI 将事件转换成第一人称提示，Agent 使用 MCP 查询上下文、管理待办并提交处理回执；Dashboard 提供完整观察和配置界面。

**Tech Stack:** Go、SQLite、Gin、Python MCP、原生 HTML/CSS/JavaScript、现有 Session Registry 与 PTY FIFO。

---

### Task 1: Pulse 持久化

**Files:**
- Modify: `src/sidecar/internal/store/sqlite.go`
- Create: `src/sidecar/internal/store/pulse_test.go`

1. 为待办、Pulse 状态和 Pulse 历史编写失败测试。
2. 运行 `go test ./internal/store`，确认因接口缺失失败。
3. 新增表、数据结构和 CRUD。
4. 再次运行测试，确认通过。

### Task 2: Pulse 调度器

**Files:**
- Create: `src/sidecar/internal/pulse/manager.go`
- Create: `src/sidecar/internal/pulse/manager_test.go`
- Modify: `src/sidecar/cmd/sidecar/main.go`

1. 测试到期待办优先、安静时间、冷却、每日预算和单个活动 Pulse。
2. 实现可注入时钟的 `RunOnce` 和后台扫描。
3. 复用 `session.Registry.Publish` 发布 `pulse` 事件。
4. 在 Sidecar 生命周期中启动和停止 Manager。

### Task 3: Sidecar API 与 MCP

**Files:**
- Modify: `src/sidecar/internal/local/api_server.go`
- Create: `src/sidecar/internal/local/pulse.go`
- Create: `src/sidecar/internal/local/pulse_test.go`
- Create: `src/mcp-server/tools/pulse.py`
- Create: `src/mcp-server/test_pulse.py`
- Modify: `src/mcp-server/server.py`

1. 测试待办创建、列表、完成、延后，Pulse 上下文、回执和配置更新。
2. 实现身份隔离的本地 API。
3. 暴露 `pingo_todo_*`、`pingo_pulse_context`、`pingo_pulse_complete` 和 `pingo_update_pulse_settings`。

### Task 4: CLI 唤醒

**Files:**
- Modify: `src/pingo/cmd/pingo/claude_pty.go`
- Modify: `src/pingo/cmd/pingo/wake_test.go`
- Modify: `src/pingo/cmd/pingo/main.go`

1. 测试 `pulse` SSE 事件可识别且生成第一人称短提示。
2. 将 Pulse 进入既有 FIFO，不增加新的忙闲状态。
3. 在会话背景中声明自主循环与回执规则。

### Task 5: Dashboard 与文档

**Files:**
- Modify: `src/sidecar/internal/dashboard/static/index.html`
- Modify: `skills/pingo/SKILL.md`
- Modify: `docs/agent/usage.md`
- Modify: `docs/reference/mcp-tools.md`
- Modify: `docs/reference/sidecar-api.md`

1. 新增“自主活动”导航、配置、待办和历史界面。
2. 更新 Skill，要求 Agent 自主维护待办、资料、关系和探索活动。
3. 补齐 MCP/API 文档和安全边界。
4. 运行 JavaScript 语法检查、全量测试、构建和打包。

