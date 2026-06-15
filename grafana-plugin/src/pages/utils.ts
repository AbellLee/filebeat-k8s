import { parse, stringify } from 'yaml';
import { t } from '@grafana/i18n';
import { Agent, Policy } from '../types';

const healthyHeartbeatMs = 5 * 60 * 1000;
const clockSkewGraceMs = 30 * 1000;

export const emptyPolicy = (): Policy => ({
  id: '',
  name: '',
  cluster_id: 'dev',
  namespace: '',
  controller_type: '',
  controller_name: '',
  container_name: '',
  node_selector: '',
  log_type: 'container_stdio',
  log_path: '',
  enabled: true,
  priority: 100,
  current_revision: 0,
  custom_fields: {},
  input_config: {},
});

export function formatDate(value?: string): string {
  if (!value) {
    return '-';
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString();
}

export function timeAgo(value?: string): string {
  if (!value) {
    return '-';
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  const ageMs = Date.now() - date.getTime();
  if (ageMs < -clockSkewGraceMs) {
    return t('filebeat-k8s-app.common.clockSkew', 'clock skew');
  }
  const seconds = Math.max(1, Math.round(Math.max(0, ageMs) / 1000));
  if (seconds < 60) {
    return t('filebeat-k8s-app.common.secondsAgo', '{{count}}s ago', { count: seconds });
  }
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) {
    return t('filebeat-k8s-app.common.minutesAgo', '{{count}}m ago', { count: minutes });
  }
  return t('filebeat-k8s-app.common.hoursAgo', '{{count}}h ago', { count: Math.round(minutes / 60) });
}

export function policyScope(policy: Policy): string {
  if (policy.log_type === 'host_file') {
    return policy.log_path || '-';
  }
  return [policy.namespace, policy.controller_type, policy.controller_name, policy.container_name]
    .filter(Boolean)
    .join(' / ');
}

export function derivePolicyName(policy: Policy): string {
  const clusterID = policy.cluster_id.trim();
  const namespace = (policy.namespace || '').trim();
  const controllerType = (policy.controller_type || '').trim();
  const controllerName = (policy.controller_name || '').trim();
  const containerName = (policy.container_name || '').trim();
  const logType = policy.log_type.trim();
  const logPath = (policy.log_path || '').trim();

  if (logType === 'host_file') {
    return clusterID && logPath ? [clusterID, 'host_file', logPath].join(' / ') : '';
  }

  if (!namespace || !controllerType || !controllerName || !containerName) {
    return '';
  }
  if (logType === 'container_file' && !logPath) {
    return '';
  }

  const parts = [namespace, `${controllerType}/${controllerName}`, containerName, logType];
  if (logType === 'container_file') {
    parts.push(logPath);
  }
  return parts.join(' / ');
}

export function uniqueClusterIds(agents: Agent[]): string[] {
  return Array.from(new Set(agents.map((agent) => agent.cluster_id.trim()).filter(Boolean))).sort();
}

export function shortChecksum(value?: string): string {
  if (!value) {
    return '-';
  }
  if (!value.startsWith('sha256:') || value.length < 24) {
    return value;
  }
  return `${value.slice(0, 13)}...${value.slice(-6)}`;
}

export function customFieldsToText(fields?: Record<string, string>): string {
  return Object.entries(fields ?? {})
    .map(([key, value]) => `${key}=${value}`)
    .join('\n');
}

export function customFieldsFromText(value: string): Record<string, string> {
  const fields: Record<string, string> = {};
  for (const line of value.split('\n')) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith('#')) {
      continue;
    }
    const index = trimmed.indexOf('=');
    if (index <= 0) {
      continue;
    }
    fields[trimmed.slice(0, index).trim()] = trimmed.slice(index + 1).trim();
  }
  return fields;
}

const reservedInputConfigKeys = new Set([
  'type',
  'id',
  'enabled',
  'paths',
  'parsers',
  'processors',
  'prospector.scanner.symlinks',
]);

export function inputConfigToYaml(config?: Record<string, unknown>): string {
  if (!config || Object.keys(config).length === 0) {
    return '';
  }
  return stringify(config, { lineWidth: 0 }).trimEnd();
}

export function inputConfigFromYaml(value: string): Record<string, unknown> {
  const trimmed = value.trim();
  if (!trimmed) {
    return {};
  }
  const parsed = parse(trimmed) as unknown;
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new Error(t('filebeat-k8s-app.errors.inputConfigObject', 'input_config must be a YAML object/map'));
  }
  const config = parsed as Record<string, unknown>;
  for (const key of Object.keys(config)) {
    const normalized = key.trim();
    if (!normalized) {
      throw new Error(t('filebeat-k8s-app.errors.inputConfigEmptyKey', 'input_config cannot contain an empty key'));
    }
    if (reservedInputConfigKeys.has(normalized)) {
      throw new Error(
        t('filebeat-k8s-app.errors.inputConfigReserved', 'input_config cannot override reserved field {{field}}', {
          field: normalized,
        })
      );
    }
  }
  return config;
}

export function agentHealthy(agent: Agent): boolean {
  if (!agent.last_heartbeat_at) {
    return false;
  }
  const lastHeartbeat = new Date(agent.last_heartbeat_at).getTime();
  if (Number.isNaN(lastHeartbeat)) {
    return false;
  }
  const ageMs = Date.now() - lastHeartbeat;
  return ageMs >= -clockSkewGraceMs && ageMs < healthyHeartbeatMs;
}
