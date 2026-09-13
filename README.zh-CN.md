# NodeStatus Go

[English](./README.md) | [简体中文](./README.zh-CN.md)

[NodeStatus](https://github.com/nodestatus/nodestatus) 服务端的一个轻量级、高性能的 Go 语言重写版本。

## 📖 简介
NodeStatus Go 旨在提供一个资源占用更低、运行速度更快的 Node.js 替代方案。它专注于后端 API，默认使用 SQLite 存储数据，并由 NezhaDash 等外部面板管理。

## ✨ 特性
- **轻量且高效**: 基于 Go 构建，内存占用低，性能优异。
- **SQLite 数据库**: 采用 SQLite 方便部署和管理。无缝复用原 Node 版的数据库表结构（`servers`、`options`、`events`、`server_histories`）。
- **核心 API 支持**:
  - 供外部面板使用的管理端 API
  - 探针客户端 `/connect` WebSocket 通信
  - 前端 `/public` WebSocket 实时数据推送
  - 状态、历史记录和事件查询

*注：作为精简版，目前不内置管理前端，也暂不支持 Telegram 机器人推送、IPC、MySQL/PostgreSQL 数据库或旧版公共主题。*

## 🚀 快速开始

### 环境要求
- [Go](https://go.dev/) 1.24+

### 构建后端程序
```sh
cd nodestatus-go
go build -o nodestatus-go ./cmd/nodestatus-go
```

## ⚙️ 运行与配置

你可以通过环境变量来配置服务的运行参数。默认配置已尽量与原 Node 版本保持一致。

```sh
WEB_PASSWORD=你的安全密码 ./nodestatus-go
# 或者通过 go run 直接运行
WEB_PASSWORD=你的安全密码 go run ./cmd/nodestatus-go
```

### 环境变量
| 变量名 | 默认值 | 描述 |
| :--- | :--- | :--- |
| `PORT` | `35601` | 服务监听的端口号。 |
| `DATABASE` | `$HOME/.nodestatus/db.sqlite` | SQLite 数据库路径 (支持普通路径或 `file:` URL)。 |
| `WEB_USERNAME` | `admin` | 管理面板的登录用户名。 |
| `WEB_PASSWORD` | *(必填)* | 管理面板的登录密码。 |
| `WEB_SECRET` | `node-secret` | JWT 或 Session 的加密密钥。 |
| `INTERVAL` | `1500` | 前端数据刷新间隔 (毫秒)。 |
| `PING_INTERVAL` | `30` | WebSocket Ping 间隔 (秒)。 |
| `RECONNECT_TIMEOUT`| `120` | 探针断线重连超时时间 (秒)。 |

## 🔌 API 接口概览
当前支持以下核心接口。其中管理端 API 保持兼容外部面板的 `{ code, msg, data }` 响应格式。

- `POST /api/admin/session` - 管理员登录鉴权
- `GET/PUT /api/admin/servers` - 服务器节点管理
- `PUT /api/admin/servers/order` - 更新服务器节点排序
- `GET /api/admin/events` - 获取事件日志
- `GET /api/config` - 获取公共配置信息
- `GET /api/status` - 获取所有服务器的实时状态
- `GET /api/server/:username/history` - 获取指定服务器的历史记录
- `WS /connect` - 供探针客户端连接的 WebSocket 接口
- `WS /public` - 供前端实时展示状态的 WebSocket 接口
