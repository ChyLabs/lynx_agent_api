# lynx_agent_api

The node agent for Lynx Panel. Provisions and manages LXC containers with enforced resource limits and live metric streaming.

---

## Overview

`lynx_agent_api` runs directly on each LXC host node. It is responsible for the full container lifecycle — provisioning, start, stop, restart, deletion, and resource updates — and streams real-time CPU, memory, disk, and network metrics to the panel over WebSocket.

Resource isolation is enforced at the kernel level:

- CPU and memory limits via cgroup v2 (`lxc.cgroup2.*`)
- Disk quotas via loop-mounted ext4 images (hard limit enforced by the filesystem)
- Swap bounded to the memory limit to prevent host spillover

All requests to the management API are authenticated by a node API key derived from the host machine ID.

---

## Requirements

| Requirement | Version |
|---|---|
| OS | Ubuntu 24.04 LTS |
| Go | 1.25.0+ (CGO enabled) |
| liblxc | liblxc-dev |
| cgroup version | v2 |
| Redis | Any recent version |

> See [INSTALL.md](../INSTALL.md) for full host setup instructions including LXC services, IP forwarding, and bridge configuration.

---

## Project Structure

```
src/
├── config/               Configuration loader (YAML)
├── infra/
│   ├── db/               SQLite (GORM) + Redis initialisation
│   └── schemas/          Database models
├── lib/                  Shared helpers (response, error, crypto, exec)
├── middleware/           Node API key authentication
├── network/
│   ├── controllers/      Request handlers
│   │   ├── server.controller.go    Container lifecycle
│   │   ├── resources.controller.go WebSocket metrics
│   │   ├── node.controller.go      Host-level metrics
│   │   └── ssh.controller.go       WebSocket terminal
│   └── routes/           Route registration
└── utils/                ID and UUID generation
```

---

## Configuration

The agent reads `config.yml` from the project root in development and from `/var/lynx/config.yml` in production. The path can be overridden with the `CONFIG_PATH` environment variable.

```yaml
server:
  port: "8080"
  env: "development"         # development | production
  api_base_route: "/api/v1"

backend:
  api_url: "http://localhost:5000/api/v1"

database:
  path: "./data/agent.db"
  redis:
    addr: "localhost:6379"
    password: ""

security:
  api_key_secret: "your-secret-key"

cors:
  allowed_origins: "http://localhost:3000,http://localhost:5173"

lxc:
  bridge: "lxcbr0"           # lxcbr0 for NAT, br0 for bridged (multiple IPs)
```

| Key | Description |
|---|---|
| `server.port` | Port the agent listens on |
| `server.env` | Controls config file path (`production` -> `/var/lynx/config.yml`) |
| `security.api_key_secret` | Secret used to decrypt the `X-Node-Api-Key` header |
| `lxc.bridge` | Network bridge containers attach to |
| `database.redis.addr` | Redis address used for sequential SID allocation |

---

## Authentication

All routes under `/api/v1` require a `X-Node-Api-Key` header. The middleware decrypts the key and compares the embedded machine ID against the host's `/etc/machine-id`. Requests with a missing, malformed, or mismatched key are rejected with `401`.

WebSocket endpoints (`/resources`, `/node`, `/ssh`) do not require the header and are intended to be called from the panel backend only.

---

## API Reference

### Servers — `/api/v1/servers`

All routes require `X-Node-Api-Key`.

| Method | Path | Description |
|---|---|---|
| GET | `/list` | List all servers |
| GET | `/{uuid}` | Get server by UUID |
| GET | `/node/{node_uuid}` | List servers by node |
| GET | `/user/{user_uuid}` | List servers by user |
| GET | `/allocation/{allocation_uuid}` | Get server by allocation |
| POST | `/provision` | Provision a new server |
| PATCH | `/update/{uuid}` | Update CPU or memory |
| PATCH | `/start/{uuid}` | Start server |
| PATCH | `/stop/{uuid}` | Stop server |
| PATCH | `/restart/{uuid}` | Restart server |
| DELETE | `/delete/{uuid}` | Delete server and all data |

#### POST `/api/v1/servers/provision`

```json
{
  "name": "my-server",
  "distro": "ubuntu",
  "release": "noble",
  "archi": "amd64",
  "memory": 2048,
  "cpu": 2,
  "disk": 10240,
  "user_id": "user-uuid",
  "node_id": "node-uuid",
  "password": "root-password",
  "allocation_ip": "10.0.3.10",
  "allocation_id": "allocation-uuid"
}
```

Provisioning is asynchronous. The response returns immediately with status `INSTALLING`. Poll `GET /{uuid}` to track progress. On success the status transitions to `RUNNING`.

`allocation_ip` and `allocation_id` are optional. When omitted the container uses the bridge's DHCP.

#### PATCH `/api/v1/servers/update/{uuid}`

```json
{
  "memory": 4096,
  "cpu": 4
}
```

Both fields are optional. The container is stopped, reconfigured, and restarted automatically.

---

### Resources — `/resources`

No authentication required.

| Method | Path | Description |
|---|---|---|
| GET (WS) | `/{uuid}/ws` | Live container metrics stream |

WebSocket messages are sent every second. Example payload when running:

```json
{
  "server_id": "uuid",
  "name": "my-server",
  "running": true,
  "cpu_percent": 12.4,
  "memory_usage": 39845888,
  "disk_usage": 154009600,
  "interface_stats": {
    "eth0": { "rx_bytes": 1024, "tx_bytes": 512 }
  },
  "timestamp": 1749500000
}
```

When the container is stopped all metric values are `0`.

`memory_usage` and `disk_usage` are in bytes.

---

### Node — `/node`

| Method | Path | Description |
|---|---|---|
| GET (WS) | `/resources/ws` | Live host-level metrics stream |

---

### SSH Terminal — `/ssh`

| Method | Path | Description |
|---|---|---|
| GET (WS) | `/{uuid}/ws` | Interactive terminal session for the container |

---

## Building

CGO must be enabled and `liblxc-dev` must be installed.

```bash
CGO_ENABLED=1 go build -o lynx-agent ./src
```

---

## Running

```bash
# Development (reads ./config.yml)
./lynx-agent

# Production (reads /var/lynx/config.yml)
ENV=production ./lynx-agent

# Custom config path
CONFIG_PATH=/etc/lynx/agent.yml ./lynx-agent
```

---

## Running as a Service

A systemd unit file is provided at `systemd/lynx-agent.service`.

#### Install

```bash
# Copy the binary to the expected location
cp lynx-agent /usr/local/bin/lynx_agent

# Copy the config to the production path
mkdir -p /var/lynx
cp config.yml /var/lynx/config.yml

# Install the service unit
cp systemd/lynx-agent.service /etc/systemd/system/lynx-agent.service
systemctl daemon-reload
```

#### Enable and start

```bash
systemctl enable lynx-agent
systemctl start  lynx-agent
```

#### Check status and logs

```bash
systemctl status lynx-agent
journalctl -u lynx-agent -f
```

#### Service behaviour

| Setting | Value | Notes |
|---|---|---|
| `User` / `Group` | `root` | Required — LXC container management needs root |
| `WorkingDirectory` | `/usr/local/bin` | Binary and working directory |
| `Environment` | `ENV=production` | Causes the agent to read `/var/lynx/config.yml` |
| `Restart` | `always` | Agent restarts automatically after any failure |
| `RestartSec` | `3s` | Wait 3 seconds before each restart attempt |
| `NoNewPrivileges` | `true` | Prevents privilege escalation via setuid/setgid |
| `PrivateTmp` | `true` | Isolates `/tmp` from other services |
| `ProtectSystem` | `full` | Mounts `/usr`, `/boot`, `/efi` read-only |
| `ProtectHome` | `true` | Blocks access to `/home`, `/root`, `/run/user` |
| `LimitNOFILE` | `65535` | Raised file descriptor limit for many concurrent containers |

> The service starts only after `network-online.target` to ensure the LXC bridge is up before any container operations run.

---

## Supported Distros

| Distro | Init System | Notes |
|---|---|---|
| Ubuntu | systemd | Default and most tested |
| Debian | systemd | |
| Alpine | OpenRC | No systemd, uses busybox init |
| Arch Linux | systemd | Full system upgrade run before install |
| CentOS / Rocky / Alma / RHEL | systemd | dnf preferred, yum fallback |
| Fedora | systemd | dnf only |
| openSUSE / SUSE | systemd | |

Any distro available via `lxc-download` can be provisioned by passing the appropriate `distro`, `release`, and `archi` values.
