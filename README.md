# Filebeat-Ops Control Server + Sidecar

This repository implements the Filebeat-Ops control plane described in:

- `control-server-reproduction-guide.md`
- `sidecar-implementation-guide.md`

## Components

- `server/cmd/control-server`: Control Server with Policy CRUD, revisions, Agent APIs, config/watch, apply-result tracking, Kubernetes resource discovery, and optional `FilebeatPolicy` Operator.
- `sidecar/cmd/control-sidecar`: node-side agent that registers, heartbeats, pulls desired config, atomically writes `inputs.d/fbctl-*.yml`, reports apply results, and maintains `/var/log/klog` plus `/var/log/klog-stdio` views.
- `internal/control`: shared API types, policy validation, selector matching, renderer, filenames, and checksum logic.

## Local Checks

```powershell
go test ./...
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\verify-containers.ps1
```

The full local loop requires Docker:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\verify-basic.ps1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\verify-basic.ps1 -ConfigMode watch -Port 18082 -MySQLPort 53307
```

## K3s Smoke Test

When running inside a local WSL k3s cluster as root:

```bash
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
kubectl apply -f deploy/kubernetes/smoke/k3s-smoke-base.yaml
scripts/k3s-smoke.sh
```

The script creates a policy, runs the sidecar once, verifies `fbctl-*.yml` is written, deletes the policy, and verifies orphan cleanup.

## Docker Compose

```powershell
docker compose up --build mysql control-server
docker compose up --build
```

Default Control Server URL:

```text
http://localhost:18080
```

## Kubernetes log path adaptation

Policies keep using stable Filebeat paths managed by the sidecar:

- `container_stdio`: `/var/log/klog-stdio/{namespace}/{controllerType}/{controllerName}/*/containers/{container}/*.log`
- `container_file`: `/var/log/klog/{namespace}/{controllerType}/{controllerName}/*/containers/{container}/{logPath}`

The sidecar auto-detects the Kubernetes profile, container runtime, stdout log directory, and container rootfs capability on each node. `container_stdio` is the recommended cross-cloud path because it is backed by node stdout logs such as `/var/log/containers/*.log`. `container_file` is best-effort: it requires resolving the container rootfs through `/proc/<pid>/root` or containerd state. When rootfs access is unavailable, the agent reports `container_file=degraded/unsupported` and Grafana shows the reason.

Useful sidecar env vars:

- `K8S_PROFILE=auto`: `auto|generic|ack|eks|gke|aks|tke`
- `CONTAINER_FILE_MODE=auto`: `auto|disabled|required`
- `STDIO_LOG_DIR_CANDIDATES`: comma-separated stdout log directories
- `CONTAINERD_STATE_DIR_CANDIDATES`: comma-separated containerd state directories

## Kubernetes

Build local images:

```powershell
docker build -f deploy/docker/server.Dockerfile -t filebeat-ops-server:local .
docker build -f deploy/docker/sidecar.Dockerfile -t filebeat-ops-sidecar:local .
```

Render or apply the base manifests:

```powershell
kubectl kustomize deploy/kubernetes/base
kubectl apply -k deploy/kubernetes/base
```

Before production use, replace the example Secret values in `deploy/kubernetes/base/secret.yaml`.
