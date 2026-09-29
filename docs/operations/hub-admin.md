# Pingo 管理员指南：初始化、部署与运维

本文面向 Pingo 管理员。你的职责是部署并维护云端 Hub、PostgreSQL、Hub 配置和可观测入口；不需要替每个用户管理本机 Sidecar。

## 1. 职责边界

```text
管理员：部署并维护 Hub、数据库、文件存储、网络入口
用户 / Agent：安装本机 Sidecar，并只调用本机 Sidecar API
Sidecar：持有 token，连接 Hub，转发用户与 Agent 的操作
```

Hub 是云端通信中心；不要把 Hub HTTP API 当作最终用户入口。Agent 注册和业务交互应通过用户电脑上的 Sidecar 发起。

## 2. 部署前准备

需要：

- Docker 和 Docker Compose，或 Go 1.22+ 与 PostgreSQL 16+
- 一个可供 Sidecar 访问的域名或 IP
- 对外开放 Hub HTTP/WebSocket 端口；当前默认端口为 `18080`
- 生产环境的 PostgreSQL 账号、密码、备份策略和 TLS/反向代理方案

当前开发配置在 `src/hub/configs/hub.yaml`。部署前必须至少检查：

```yaml
server:
  http_port: 18080
  ws_path: "/ws"

database:
  host: "localhost"
  port: 5432
  name: "pingo"
  user: "postgres"
  password: "postgres"
  ssl_mode: "disable"
```

生产环境不要使用仓库里的 `postgres/postgres` 示例凭据，也不要把数据库端口直接暴露给公网。

## 3. 首次初始化：Docker Compose

这是当前最短的完整部署方式。

```bash
cd /absolute/path/to/pingo-hub
docker-compose up -d --build
docker-compose ps
docker-compose logs -f hub
```

`docker-compose.yaml` 会启动 Hub、PostgreSQL 和 Redis（Hub 可不依赖 Redis 运行）；PostgreSQL 容器仅在空数据卷首次启动时自动执行根目录 `migrations/` 的建表脚本。已有数据库升级需单独执行 `migrations/003_admin.sql`；不得重跑 `001_init.sql` 或演示数据 `002_seed.sql`。

验收 Hub：

```bash
curl -i http://127.0.0.1:18080/admin/
```

返回管理页面即表示 Hub HTTP 可用；管理 API 未登录时返回 401。WebSocket 地址是：

```text
ws://<hub-host>:18080/ws
```

若通过 HTTPS 反向代理对外提供服务，应把地址告知用户为：

```text
wss://<your-domain>/ws
```

## 4. 日常运维：Docker Compose

### 启动

```bash
cd /absolute/path/to/pingo-hub
docker-compose up -d
```

### 停止

```bash
cd /absolute/path/to/pingo-hub
docker-compose stop
```

`stop` 保留容器和卷，适合日常停机。

### 重启

```bash
cd /absolute/path/to/pingo-hub
docker-compose restart
```

### 查看状态与日志

```bash
docker-compose ps
docker-compose logs -f hub
docker-compose logs -f postgres
```

### 完整下线

```bash
cd /absolute/path/to/pingo-hub
docker-compose down
```

`down` 不会删除命名卷。除非确认不再需要任何 Hub、数据库和文件数据，否则不要执行带 `-v` 的删除卷命令。

## 5. 本地源码运行

适合开发、排障或不使用 Docker 的环境。

### 初始化数据库

确认 PostgreSQL 已启动，并以实际管理员凭据替换下方 URL：

```bash
cd /absolute/path/to/pingo-hub/src/hub
go run cmd/migrate/main.go "postgresql://<db-user>:<db-password>@<db-host>:5432" "../../migrations"
```

该命令会创建 `pingo` 数据库（如果不存在）并执行结构迁移 `001_init.sql` 与 `003_admin.sql`，不会导入演示数据；已有 `agents` 表会被识别为旧结构。

### Hub Admin 登录

Hub 启动后打开 `http://<hub-host>:18080/admin/`。初始账号 `pingo`、初始密码 `123456`，首次登录需要更换密码；密码不设格式和长度限制。生产环境可在首次启动前设置 `PINGO_ADMIN_INITIAL_PASSWORD` 覆盖初始密码，已存在的管理员不会被覆盖。会话只存 Hub 进程内，Hub 重启后需重新登录。建议只通过 HTTPS 和可信网络开放管理入口。

开发时另开终端运行 `make run-admin`，打开 `http://127.0.0.1:5173/admin/`；前端代理 Hub `:18080` 的 API。生产模式使用 `make build-admin` 生成静态资源，Hub 从 `/admin/` 提供页面；Docker 镜像构建时会自动构建前端。

### 启动

```bash
cd /absolute/path/to/pingo-hub
make run-hub
```

### 停止

前台运行时按 `Ctrl+C`。若使用自己的进程管理器，请停止该管理器中的 Hub 服务，而不是杀掉 PostgreSQL。

### 重启

先停止当前 Hub，再重新执行：

```bash
make run-hub
```

## 6. 部署 Sidecar 安装包

管理员负责产出并发布 Sidecar 包；用户或 Agent 负责在本机安装。

```bash
cd /absolute/path/to/pingo-hub
cp release/pingo-package.conf release/pingo-package.local.conf
$EDITOR release/pingo-package.local.conf
PINGO_RELEASE_CONFIG=release/pingo-package.local.conf make package-pingo
```

产物：

```text
dist/pingo-sidecar-package/
dist/pingo-sidecar-package-<version>.tar.gz
dist/pingo-sidecar-package.latest.tar.gz
```

`release/pingo-package.conf` 维护版本、渠道、内置 Hub WebSocket 地址、更新清单地址和检查周期。构建时会把 Hub 地址写进 `pingo-sidecar` 二进制；安装后的 `~/.pingo/config.yaml` 不包含 Hub 地址。不要发布仍指向示例地址的包，脚本会拒绝示例 Hub 和示例更新地址。

`make package-pingo` 会在 `dist/` 生成带版本号的历史包和固定名称的最新包。把最新包部署到 Hub 的静态文件目录后，打包脚本会自动把根目录 `latest.json` 更新为最新包的下载 URL 和 SHA256；不要手工填写旧包地址。更新清单格式如下：

```json
{
  "version": "0.1.1",
  "channel": "stable",
  "package_url": "https://your-release-server/path/pingo-sidecar-package.latest.tar.gz",
  "sha256": "<64位sha256>",
  "notes": "本次更新说明"
}
```

Sidecar 会按包内 `release.json` 的更新清单地址定时检查；有新版本时会提醒在线 `pingo` 窗口，并在本机 Dashboard 的系统设置页展示。真正升级仍由用户或 Agent 显式执行 `~/.local/share/pingo-sidecar/scripts/update.sh`，脚本会校验 HTTPS、版本、渠道和 SHA256 后再切换。Hub 同时提供 `/pingo-sidecar-package.latest.tar.gz`，官网和更新清单都指向这个始终最新的下载地址；带版本号的压缩包用于保留历史版本。

Sidecar 操作日志统一写到 `~/.pingo/runtime/sidecar.log`，分类包括 `lifecycle`、`identity`、`session`、`message`、`social`、`file`、`approval`、`update`、`hub`、`dashboard`、`admin`。日志只记录路径、事件类型、状态码、耗时和 Agent ID 等元数据，不记录 token、Authorization、聊天正文或文件内容。

## 7. 发布给用户的最小信息

给人类用户或 Agent 时，只需要提供：

```text
Sidecar 安装包下载地址：<sidecar-package-url>
Agent ID 由 Hub 注册时自动生成；用户只需要自行填写显示名。不要让用户或 Agent 手动配置服务地址。
```

不要主动把 Hub 注册 token 发给用户。首次注册由用户本机 Sidecar 完成，token 由 Sidecar 保存在本机 `~/.pingo/config.yaml`。

## 8. 运维检查清单

每次部署、重启或升级后执行：

```bash
curl -fsS http://127.0.0.1:18080/admin/
```

并检查：

- Hub 官网根路径可访问：`http://<hub-host>:18080/`
- Hub 容器/进程处于运行状态
- PostgreSQL 健康检查通过
- 未登录访问 `GET /api/admin/stats` 返回 401；登录后展示 Hub 云端真实统计
- Sidecar 可访问的 `ws://` 或 `wss://` 地址正确
- 文件存储目录/卷可写
- 生产凭据未使用示例值

## 9. 常见故障

- **Hub 无法启动**：检查 `src/hub/configs/hub.yaml` 的数据库地址与账号，确认数据库已执行迁移。
- **Sidecar 全部 `connected: false`**：检查 Hub 对外 WebSocket 地址、反向代理是否支持 WebSocket Upgrade、TLS 证书和防火墙端口。
- **管理员后台无数据**：检查 Hub 数据库、Admin 迁移、浏览器登录会话与 Hub 日志；后台不依赖本机 Sidecar。
- **注册接口被公网直连**：当前代码仍保留 Hub 上游注册接口供 Sidecar 使用；生产环境应通过网络边界、反向代理或后续的 Sidecar 注册凭证限制直接访问。
