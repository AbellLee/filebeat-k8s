#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
NS="${NS:-fbops-codex-smoke}"
PORT="${PORT:-18080}"
export KUBECONFIG="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"

cleanup_pf() {
  if [[ -n "${PF_PID:-}" ]]; then
    kill "${PF_PID}" >/dev/null 2>&1 || true
  fi
}
trap cleanup_pf EXIT

kubectl -n "$NS" port-forward svc/filebeat-control-server "${PORT}:8080" >/tmp/fbops-smoke-port-forward.log 2>&1 &
PF_PID=$!

for _ in {1..60}; do
  if curl -fsS "http://127.0.0.1:${PORT}/readyz" >/dev/null; then
    break
  fi
  sleep 1
done
curl -fsS "http://127.0.0.1:${PORT}/readyz" >/dev/null

cat >/tmp/fbops-policy.json <<'JSON'
{
  "id": "payment-app",
  "name": "payment app",
  "cluster_id": "dev",
  "namespace": "payment",
  "controller_type": "deployment",
  "controller_name": "payment-api",
  "container_name": "app",
  "node_selector": "smoke=true",
  "log_type": "container_stdio",
  "enabled": true,
  "priority": 100,
  "custom_fields": {
    "__project__": "cloudnet",
    "__logstore__": "payment"
  }
}
JSON

curl -fsS -X POST "http://127.0.0.1:${PORT}/api/v1/policies" \
  -H "Content-Type: application/json" \
  --data-binary @/tmp/fbops-policy.json >/tmp/fbops-policy-create.json

kubectl -n "$NS" delete pod fbops-sidecar-smoke --ignore-not-found --wait=true >/dev/null
kubectl apply -f "$ROOT/deploy/kubernetes/smoke/k3s-smoke-sidecar.yaml" >/dev/null

for _ in {1..60}; do
  phase="$(kubectl -n "$NS" get pod fbops-sidecar-smoke -o jsonpath='{.status.phase}' 2>/dev/null || true)"
  if [[ "$phase" == "Running" || "$phase" == "Succeeded" ]]; then
    break
  fi
  sleep 1
done

kubectl -n "$NS" logs fbops-sidecar-smoke --tail=80 >/tmp/fbops-sidecar.log
grep -q "applied config" /tmp/fbops-sidecar.log
kubectl -n "$NS" exec fbops-sidecar-smoke -- test -f /inputs/fbctl-100-payment-app.yml
kubectl -n "$NS" exec fbops-sidecar-smoke -- grep -Fq '/var/log/klog-stdio/payment/deployment/payment-api/*/containers/app/*.log' /inputs/fbctl-100-payment-app.yml

curl -fsS -H "X-Agent-Token: dev-agent-token" \
  "http://127.0.0.1:${PORT}/api/v1/agent/config?agent_id=dev%3Adesktop-ed6ea4n&checksum=" \
  >/tmp/fbops-agent-config.json
grep -q '"changed":true' /tmp/fbops-agent-config.json

curl -fsS -X DELETE "http://127.0.0.1:${PORT}/api/v1/policies/payment-app" >/dev/null
kubectl -n "$NS" exec fbops-sidecar-smoke -- /usr/local/bin/control-sidecar >/tmp/fbops-sidecar-cleanup.log
if kubectl -n "$NS" exec fbops-sidecar-smoke -- test -f /inputs/fbctl-100-payment-app.yml; then
  echo "orphan config was not removed" >&2
  exit 1
fi

curl -fsS "http://127.0.0.1:${PORT}/api/v1/agents" >/tmp/fbops-agents.json
grep -q '"last_apply_status":"success"' /tmp/fbops-agents.json

echo "k3s smoke passed"
