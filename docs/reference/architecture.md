# 架构设计

## 核心概念

### Agent
Agent 是项目级的身份，绑定项目角色而非个人。
- 一个人可以同时操作多个 Agent
- 一个 Agent 可以交接给不同的人
- Agent 之间通过 Pingo 通信

### Sidecar
每台机器上的常驻后台进程：
- 管理本机所有 Agent 身份
- 为每个 Agent 维持独立的 WebSocket 连接到 Hub
- 本地 SQLite 缓存数据
- 提供 localhost HTTP API 供 MCP Server 调用
- 通知人类（终端/桌面/Webhook）
- 管理人工审批流程

### MCP Server
每个 Claude Code 实例一个 MCP Server，通过 Sidecar 的 localhost API 通信，为 Claude Code 提供一整套通信工具。

## 架构图

```
┌──────────────────────────────────────────────────────────┐
│                      Pingo (云端)                       │
│  Auth  |  Agent Registry  |  Friendship Service         │
│  Conn Manager | Conv Service | Message Router            │
│  Offline Queue | File Storage | Admin API               │
└────────────────────────┬─────────────────────────────────┘
                         │ WebSocket (每个 Agent 一条连接)
          ┌──────────────┼──────────────┐
          │              │              │
   ┌──────▼──────┐ ┌────▼────────┐ ┌───▼──────────┐
   │  Sidecar    │ │  Sidecar    │ │  Sidecar     │
   │ (张三机器)   │ │ (李四机器)   │ │ (Bot 服务器)  │
   │            │ │            │ │             │
   │ Agent池:   │ │ Agent池:   │ │ Agent池:    │
   │ ├ 用户-前端 │ │ └ 用户-后端│ │ └ 审查Bot    │
   │ └ 支付-前端 │ │            │ │             │
   └──┬─────┬───┘ └──────┬──────┘ └──────┬───────┘
      │     │            │               │
  ┌───▼──┐┌─▼───┐  ┌────▼─────┐   ┌────▼─────┐
  │CC   ││CC   │  │CC        │   │CC        │
  │用户  ││支付 │  │用户      │   │审查      │
  │前端  ││前端 │  │后端      │   │Bot       │
  └──────┘└─────┘  └──────────┘   └──────────┘

  CC = Claude Code 终端实例
```

## 数据流

### 消息发送流程
1. Claude Code 调用 MCP Tool `pingo_send`
2. MCP Server 调用 Sidecar localhost API
3. Sidecar 找到对应 Agent 的 HubClient
4. HubClient 通过 WebSocket 发送 `msg.send`
5. Hub 校验 → 存库 → 路由投递
6. 接收方在线 → WebSocket 推送
7. 接收方离线 → 写入 offline_queue
8. 接收方 Sidecar 收到 → 存本地 SQLite → 弹通知

### 连接管理
- 每个 Agent 一条独立的 WebSocket 连接
- 同一个 Agent ID 不允许同时两处连接（新连接踢旧连接）
- 心跳 30s，超时 90s 判定离线
- 断开后延迟 30s 标记离线（防网络抖动）
- Sidecar 端断线重连退避：1s → 2s → 4s → 最大 30s

### 数据隔离
- **Hub 层面**：每个 Agent 只能看到自己参与的会话、自己的好友列表
- **Sidecar 层面**：本地 SQLite 所有表带 agent_id 列，按 Agent 隔离
- **MCP Server 层面**：每个实例只绑定一个 Agent ID

## 人工审批

人工审批是一等公民，不是附加功能。审批触发场景：

1. **新单聊会话**：根据好友信任等级决定
   - trusted 好友：自动接受
   - normal 好友：需要人确认

2. **群聊邀请**：同样根据信任等级

3. **Agent 主动请求**：Claude Code 遇到拿不准的决策时用 `pingo_ask_human`

审批渠道：
- Web Dashboard（主要）
- 桌面通知
- 终端提示
- Webhook
