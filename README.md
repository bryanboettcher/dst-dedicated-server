# Don't Starve Together — Dedicated Server

A containerized DST dedicated server with a built-in process supervisor and web management dashboard.

## Quick Start

**1.** Clone the repository and add your cluster token:

```bash
git clone https://github.com/bryanboettcher/dst-dedicated-server.git
cd dst-dedicated-server
```

**2.** Paste your [Klei cluster token](https://accounts.klei.com/account/game/servers?game=DontStarveTogether) into `cluster_config/cluster_token.txt`:

```bash
echo 'YOUR_TOKEN_HERE' > cluster_config/cluster_token.txt
```

**3.** Start the server:

```bash
docker compose up -d
```

That's it. You now have an Overworld + Caves server running with a web dashboard at **http://localhost:8080**.

## What's Running

The default `docker-compose.yml` starts three containers:

| Container | Image | Purpose | Port |
|---|---|---|---|
| overworld | `ghcr.io/bryanboettcher/dst-dedicated-server` | Master shard | UDP 10999 |
| caves | `ghcr.io/bryanboettcher/dst-dedicated-server` | Caves shard | UDP 10998 |
| webui | `ghcr.io/bryanboettcher/dst-dedicated-server-webui` | Management dashboard | TCP 8080 |

Each server container runs a Go process supervisor that manages the DST binary, handles graceful shutdown (saves before stopping), and exposes HTTP health/management endpoints on port 8080.

## Web UI

The web dashboard at `http://localhost:8080` provides:

- **Live status** for each shard (state, players, uptime, region)
- **Cluster actions** — save, rollback, skip day
- **Shard controls** — start, stop, restart individual shards
- **Announcements** — broadcast messages to all connected players
- **Server logs** — real-time log viewer with shard selector
- **Console** — send arbitrary DST console commands

## Configuration

Server configuration lives in `cluster_config/`:

```
cluster_config/
  cluster.ini               # Server name, password, max players, game mode
  cluster_token.txt         # Your Klei authentication token
  Overworld/
    server.ini              # Master shard settings
    leveldataoverride.lua   # World generation overrides
  Caves/
    server.ini              # Caves shard settings
    leveldataoverride.lua   # Cave generation overrides
```

Edit `cluster_config/cluster.ini` to change the server name, password, max players, and game mode.

### Mods

Place your mod configuration in `volumes/mods/`:

- `dedicated_server_mods_setup.lua` — which Workshop mods to download
- `modoverrides.lua` — mod configuration

See `cluster_config/mods/` for examples.

## Supervisor HTTP API

Each server container exposes a management API on port 8080:

| Endpoint | Description |
|---|---|
| `GET /healthz` | Liveness probe (always 200) |
| `GET /readyz` | Readiness probe (200 when DST is accepting players) |
| `GET /status` | JSON server status (state, players, uptime) |
| `GET /metrics` | Prometheus metrics |
| `POST /api/save` | Trigger world save |
| `POST /api/shutdown` | Graceful save and shutdown |
| `POST /api/restart` | Restart the DST process without restarting the container |
| `POST /api/rollback/{days}` | Roll back the world |
| `POST /api/console` | Send a console command (body = command text) |
| `GET /api/logs` | Recent log lines as JSON (?lines=N) |
| `GET /api/logs/stream` | Live log stream (SSE) |

Set `DST_ADMIN_TOKEN` to require a bearer token for management endpoints.

## Examples

The `examples/` directory has alternative deployment configurations:

- **`docker-compose.server-only.yml`** — two shards, no web UI

## Kubernetes

The server image works with Kubernetes liveness, readiness, and startup probes:

```yaml
livenessProbe:
  httpGet: { path: /healthz, port: 8080 }
readinessProbe:
  httpGet: { path: /readyz, port: 8080 }
startupProbe:
  httpGet: { path: /startupz, port: 8080 }
  failureThreshold: 30
  periodSeconds: 10
```

The web UI connects to supervisor instances via `DST_BACKENDS`:

```
DST_BACKENDS=Overworld=http://overworld-svc:8080,Caves=http://caves-svc:8080
```

## Building From Source

```bash
docker build -t dst-server .
docker build -t dst-webui ./webui
```

## License

[MIT](LICENSE.md)
