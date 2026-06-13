# Log Collection

Grafana App Plugin for managing the filebeat-k8s control plane.

The plugin provides:

- Policy CRUD and enable/disable actions.
- Server-side YAML render preview.
- Policy detail and revision rollback.
- Agent heartbeat and apply-result visibility.
- Plugin settings for `controlServerUrl` and optional `adminToken`.

In local development, build the frontend and backend before starting Grafana:

```bash
npm install
npm run build
go run github.com/magefile/mage -v build:linux
docker compose up --build mysql control-server grafana
```
