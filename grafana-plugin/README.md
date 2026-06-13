# Log Collection

Grafana App Plugin for the filebeat-k8s control plane.

## Features

- Policy list, create, edit, enable, disable, and delete.
- Server-side YAML preview through `POST /api/v1/policies/render-preview`.
- Policy detail, rendered config, revision history, and rollback.
- Agent heartbeat, checksum, node labels, and apply-result visibility.
- Plugin settings for `controlServerUrl` and optional `adminToken`.

## Local Development

Build the plugin before starting Grafana:

```powershell
cd .\grafana-plugin
npm install
npm run build
go run github.com/magefile/mage -v build:linux
cd ..
docker compose up --build mysql control-server grafana
```

Open Grafana at <http://localhost:3000>. The development provisioning points the plugin at:

```text
http://control-server:8080
```
