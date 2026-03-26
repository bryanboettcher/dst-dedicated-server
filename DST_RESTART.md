# DST Dedicated Server Image - Session State

## Current Branch: `dst`

## What This Project Is
One-shard-per-container Docker image for Don't Starve Together dedicated servers. Fork of mathielo/dst-dedicated-server, redesigned for Kubernetes deployment where each shard runs as a separate pod (DST server process is single-threaded, so multiple shards = load distribution).

## Image Architecture
- **Base:** `steamcmd/steamcmd:debian`
- **Entrypoint flow:** `entrypoint.sh` (root) → `prepare.sh` (set UID/GID) → `install.sh` (steamcmd update + mod setup, as dst user) → `run.sh` (launch server, as dst user)
- **Volumes:** `/dst/config` (cluster config + saves), `/dst/mods` (mod files), `/opt/dst_server` (game install)
- **Key env vars:** `CLUSTER_NAME`, `SHARD_NAME`, `DST_UID`, `DST_GID`

## Completed Fixes
1. **Dockerfile:** Removed `USER dst` (entrypoint needs root for prepare.sh usermod/groupmod)
2. **Dockerfile:** Split monolithic RUN into setup + install steps, added `chmod +x`
3. **Dockerfile + install.sh:** Fixed steamcmd arg order (`+force_install_dir` must come before `+login anonymous`)

## Current Blocker: steamcmd build failure
- `steamcmd` inside `docker build` RUN step fails with `ERROR! Failed to install app '343050' (Missing configuration)`
- Arg order fix didn't resolve it
- **Next step:** Run `steamcmd` interactively in a container from the base image to debug. Try:
  - `docker run --rm -it steamcmd/steamcmd:debian bash` then run steamcmd manually
  - Check if it's a Docker build layer/network issue vs actual steamcmd problem
  - The base image's ENTRYPOINT is `steamcmd` - check if there's init setup that gets skipped in RUN
  - Consider moving DST install to entrypoint (runtime) instead of build time, since K8s will use emptyDir anyway

## Remaining Work on Image
1. **run.sh:** Launch command is commented out. Needs:
   ```bash
   cd /opt/dst_server/bin64
   exec ./dontstarve_dedicated_server_nullrenderer_x64 \
     -persistent_storage_root /dst \
     -conf_dir config \
     -cluster "$CLUSTER_NAME" \
     -shard "$SHARD_NAME"
   ```
2. **install.sh:** `dedicated_server_mods_setup.lua` copy destination is wrong - copies to `$MODS_ROOT` (cluster config dir) but should go to `$INSTALL_ROOT/mods/` (server install dir, where steamcmd looks for it)
3. **entrypoint.sh:** Missing `export INSTALL_ROOT=/opt/dst_server`
4. **GitHub Actions workflow:** Create `.github/workflows/build.yml` for GHCR push

## Kubernetes Deployment Plan (after image works)
- **Namespace:** `gaming` (existing, used by Valheim)
- **Architecture:** 4 Deployments (one per shard), each running this image with different SHARD_NAME
- **Shards:** Master (forest), Caves1, Forest2, Caves2
- **Storage:** 1 PVC (performance, 20Gi, snapshot-tier: critical) for config+saves, emptyDir for server binaries
- **Networking:** Single MetalLB IP, ports 10999-11002/UDP, inter-shard comm on localhost-equivalent via Services
- **Config:** ConfigMaps for cluster.ini, server.ini per shard, worldgenoverride.lua per shard, modoverrides.lua
- **Secret:** SealedSecret for cluster token (`pds-g^KU_7veFK52b^...`)
- **Server name:** "pants-free dedicated server", password: "9704!", endless mode
- **Resources per shard:** requests 1000m/1536Mi, limits 3000m/3072Mi
- **Mods (selected):** Combined Status (376333686), Geometric Placement (351325790) - more TBD
- **Helm chart location:** `charts/apps/games/dst/` in homelab repo
- **ArgoCD:** Add to `kubernetes/argocd/applicationsets/games.yaml`
