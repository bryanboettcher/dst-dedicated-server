# DST Dedicated Server

Don't Starve Together dedicated server with a Go process supervisor and web management UI.

## Architecture

Two container images, deployed together:

- **`ghcr.io/bryanboettcher/dst-dedicated-server`** — DST game server + Go supervisor (PID 1). One container per shard (Overworld, Caves, etc). Exposes HTTP on port 8080.
- **`ghcr.io/bryanboettcher/dst-dedicated-server-webui`** — Web dashboard sidecar. Connects to one or more supervisor instances. Serves on port 8080.

## Supervisor (port 8080)

The supervisor replaces the old shell entrypoint. It manages the DST binary lifecycle, provides health endpoints, and streams logs.

### Endpoints

| Endpoint | Method | Purpose |
|---|---|---|
| `/healthz` | GET | Liveness — always 200 while supervisor is alive |
| `/readyz` | GET | Readiness — 200 when DST is running (observer-driven) |
| `/startupz` | GET | Startup — 200 once DST binary is launched |
| `/status` | GET | JSON: state, player_count, players[], game_port, region, cluster, shard, is_master, uptime |
| `/metrics` | GET | Prometheus text exposition format |
| `/api/logs` | GET | Last N log lines as JSON array (?lines=N, default 100) |
| `/api/logs/stream` | GET | SSE stream of new log lines |
| `/api/save` | POST | Trigger c_save() |
| `/api/shutdown` | POST | Graceful save + shutdown |
| `/api/restart` | POST | Save + stop + relaunch DST (not the container) |
| `/api/rollback/{days}` | POST | c_rollback() or c_rollback(N) |
| `/api/console` | POST | Send arbitrary console command (body = command text) |
| `/api/players/sync` | POST | Trigger immediate c_listplayers() poll |

Management endpoints are token-gated via `DST_ADMIN_TOKEN` env var if set.

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `SHARD_NAME` | `Overworld` | Which shard this container runs |
| `CLUSTER_NAME` | `DST_Cluster` | Cluster directory name under /dst/config |
| `DST_ADMIN_TOKEN` | (empty) | Bearer token for management API, disabled if empty |
| `PUID` / `PGID` | (from image) | Override UID/GID for the dst user |
| `DST_HEALTH_INTERVAL` | `10` | A2S health probe interval in seconds |
| `DST_HEALTH_TIMEOUT` | `3` | A2S health probe timeout in seconds |
| `DST_PLAYER_POLL_INTERVAL` | `300` | c_listplayers() poll interval in seconds |
| `DST_PLAYER_STALE` | `720` | Seconds before unseen players are pruned |
| `DST_SAVE_DELAY` | `5` | Seconds to wait after c_save() before proceeding |
| `DST_LOG_BUFFER_SIZE` | `1000` | Number of log lines retained in the ring buffer |

### A2S Health Checking (currently inactive for DST)

The supervisor includes a standard Valve A2S_INFO UDP query implementation with challenge-response support. However, DST does not respond to A2S queries on any port — the game uses Klei's lobby API for server discovery instead. The A2S health checker runs but all probes fail silently (logged at DEBUG level). Readiness transitions are driven entirely by the observer watching DST's stdout. The A2S code is retained for potential future use if Klei enables query support, and is correct for other Source engine games.

### Volumes

| Path | Purpose |
|---|---|
| `/dst/config` | Cluster configuration (cluster.ini, shard server.ini files, saves) |
| `/dst/mods` | Mod configuration (dedicated_server_mods_setup.lua, modoverrides.lua) |
| `/opt/dst_server` | Game server installation (managed by steamcmd) |

### State Machine

`PREPARING` → `INSTALLING` → `STARTING` → `RUNNING` → `STOPPING` → `STOPPED`

Readiness transition (Starting → Running) is driven by the observer watching DST stdout:
- **Master shards:** "Server registered via geo DNS" 
- **Secondary shards:** "[Shard] secondary shard LUA is now ready!" (fires after connecting to master + world load)

Player tracking uses `c_listplayers()` polling + join/leave event parsing.

## WebUI (port 8080)

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `DST_BACKENDS` | `default=http://localhost:8081` | Comma-separated `name=url` pairs for each shard supervisor |
| `DST_WEBUI_LISTEN` | `:8080` | Listen address |

### How It Connects

The webui reverse-proxies requests to supervisor instances:
- `GET /shards` — returns configured shard names
- `/shard/{name}/*` — proxied to that shard's supervisor URL
- `GET /events` — SSE stream aggregating `/status` from all shards (5s interval)

## Project Structure

```
Dockerfile                  # Server image (multi-stage: Go build + steamcmd base)
docker-compose.yml          # Default 2-shard + webui setup (clone and run)
supervisor/                 # Go supervisor source
webui/                      # WebUI sidecar (Go + embedded SPA)
  Dockerfile                # WebUI image (multi-stage: Go build + distroless)
  mock/                     # Mock supervisor for local UI development
cluster_config/             # Example DST configuration
  cluster.ini               # Cluster settings
  cluster_token.txt         # Klei auth token (user must replace)
  Overworld/server.ini      # Master shard config
  Caves/server.ini          # Caves shard config
examples/                   # Alternative deployment configurations
```

## Building

```bash
# Server image
docker build -t dst-server .

# WebUI image
docker build -t dst-webui ./webui

# Local UI development (no DST needed)
go run ./webui/mock &    # fake supervisor on :8081
go run ./webui           # webui on :8080
```
