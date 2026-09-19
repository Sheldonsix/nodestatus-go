# NodeStatus Go

[English](./README.md) | [简体中文](./README.zh-CN.md)

A minimal, high-performance Go backend for the [NodeStatus](https://github.com/nodestatus/nodestatus) server.

## 📖 Introduction
NodeStatus Go provides a lightweight and fast alternative to the original Node.js backend. It focuses on the backend API, uses SQLite for data storage, and is intended to be managed by an external dashboard such as NezhaDash.

## ✨ Features
- **Lightweight & Fast**: Built with Go for optimal performance and low memory footprint.
- **SQLite Database**: Uses SQLite for easy setup and management. Reuses existing tables from the Node.js version (`servers`, `options`, `events`, `server_histories`).
- **Core APIs Supported**:
  - Admin API for external dashboards
  - Agent `/connect` WebSocket
  - Public `/public` WebSocket
  - Status, history, and event endpoints.

*Note: This minimal version currently does not include a bundled admin UI, Telegram push, IPC, MySQL/Postgres support, or legacy public themes.*

## 🚀 Getting Started

Prerequisite: Docker with Compose v2.

### Docker Deploy
```sh
mkdir -p ~/nodestatus-go
cd ~/nodestatus-go
curl -fsSLO https://raw.githubusercontent.com/Sheldonsix/nodestatus-go/main/docker-compose.yml

cat > .env <<'EOF'
WEB_PASSWORD=your-secure-password
WEB_SECRET=your-random-secret
BIND=127.0.0.1:35601:35601
EOF

docker compose pull
docker compose up -d
```

### Client Agent
`client-go` lives in `client-go/` as a separate Go module. Release assets are installed with:

```sh
wget -O /tmp/nodestatus-client-install.sh https://raw.githubusercontent.com/Sheldonsix/nodestatus-go/main/scripts/install-client-go.sh
sh /tmp/nodestatus-client-install.sh --dsn 'wss://node:password@example.com'
```

### GitHub Actions
- `CI`: tests server and client; can be run manually.
- `Docker Release`: builds and pushes `ghcr.io/<owner>/<repo>` and `DOCKERHUB_USERNAME/nodestatus-go`; runs on `v*` tags or manually. Set `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN` repository secrets first.
- `Client Go Release`: builds Linux/OpenWrt client archives; runs on `v*` tags or manually.

## ⚙️ Configuration

### Environment Variables
| Variable | Default Value | Description |
| :--- | :--- | :--- |
| `PORT` | `35601` | The port the server listens on. |
| `DATABASE` | `$HOME/.nodestatus/db.sqlite` | Path to the SQLite DB (can be a plain path or a `file:` URL). |
| `WEB_USERNAME` | `admin` | Admin dashboard username. |
| `WEB_PASSWORD` | *(required)* | Admin dashboard password. |
| `WEB_SECRET` | `node-secret` | Secret key for JWT/session encryption. |
| `INTERVAL` | `1500` | Data update interval in milliseconds. |
| `PING_INTERVAL` | `30` | WebSocket ping interval in seconds. |
| `RECONNECT_TIMEOUT`| `120` | Agent reconnect timeout in seconds. |

## 🔌 API Endpoints
The following core endpoints are supported. Admin API responses maintain the `{ code, msg, data }` structure used by compatible dashboards.

- `POST /api/admin/session` - Admin authentication
- `GET/PUT /api/admin/servers` - Server management
- `PUT /api/admin/servers/order` - Update server display order
- `GET /api/admin/events` - Fetch events
- `GET /api/config` - Public configuration
- `GET /api/status` - Current status of all servers
- `GET /api/server/:username/history` - Historical data for a specific server
- `WS /connect` - WebSocket for agent connections
- `WS /public` - Public WebSocket for real-time frontend updates
