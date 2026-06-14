import React, { useEffect, useMemo, useState } from 'react';
import { PluginPage } from '@grafana/runtime';
import { Alert, Button, useStyles2 } from '@grafana/ui';
import { t } from '@grafana/i18n';
import { api } from '../api/client';
import { Agent, CapabilityDetail } from '../types';
import { agentHealthy, shortChecksum, timeAgo } from './utils';
import { getPageStyles } from './styles';

function AgentsPage() {
  const s = useStyles2(getPageStyles);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [selected, setSelected] = useState<Agent | undefined>();
  const [cluster, setCluster] = useState('');
  const [error, setError] = useState('');

  const loadAgents = () => {
    api
      .listAgents()
      .then((data) => {
        setAgents(data);
        setSelected((current) => current ?? data[0]);
        setError('');
      })
      .catch((err) => setError(err.message));
  };

  useEffect(loadAgents, []);

  const clusters = Array.from(new Set(agents.map((agent) => agent.cluster_id))).sort();
  const filtered = useMemo(() => agents.filter((agent) => !cluster || agent.cluster_id === cluster), [agents, cluster]);

  return (
    <PluginPage>
      <div className={s.page}>
        <div className={s.header}>
          <div>
            <div className={s.eyebrow}>Filebeat Ops / Agents</div>
            <h1 className={s.title}>{t('filebeat-k8s-app.agents.title', 'Agent status')}</h1>
            <div className={s.subtitle}>
              {t('filebeat-k8s-app.agents.subtitle', 'Review sidecar heartbeat, apply-result, and node log collection capability.')}
            </div>
          </div>
          <div className={s.toolbar}>
            <select className={s.input} value={cluster} onChange={(event) => setCluster(event.target.value)}>
              <option value="">{t('filebeat-k8s-app.agents.clusterAll', 'cluster: all')}</option>
              {clusters.map((item) => (
                <option key={item} value={item}>
                  {item}
                </option>
              ))}
            </select>
            <Button variant="secondary" icon="sync" onClick={loadAgents}>
              {t('filebeat-k8s-app.common.refresh', 'Refresh')}
            </Button>
          </div>
        </div>

        {error && (
          <Alert title={t('filebeat-k8s-app.common.loadFailed', 'Load failed')} severity="error">
            {error}
          </Alert>
        )}

        <div className={s.split}>
          <section className={s.card}>
            <table className={s.table}>
              <thead>
                <tr>
                  <th>{t('filebeat-k8s-app.fields.agentId', 'Agent ID')}</th>
                  <th>{t('filebeat-k8s-app.fields.nodeName', 'Node name')}</th>
                  <th>{t('filebeat-k8s-app.fields.heartbeat', 'Heartbeat')}</th>
                  <th>{t('filebeat-k8s-app.fields.profile', 'Profile')}</th>
                  <th>{t('filebeat-k8s-app.fields.runtime', 'Runtime')}</th>
                  <th>{t('filebeat-k8s-app.fields.stdio', 'stdio')}</th>
                  <th>{t('filebeat-k8s-app.fields.containerFile', 'container_file')}</th>
                  <th>{t('filebeat-k8s-app.fields.lastApplyStatus', 'Last apply status')}</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((agent) => (
                  <tr key={agent.id} onClick={() => setSelected(agent)}>
                    <td className={s.mono}>{agent.id}</td>
                    <td>{agent.node_name}</td>
                    <td>
                      <span className={`${s.chip} ${agentHealthy(agent) ? s.chipGreen : s.chipOrange}`}>
                        {timeAgo(agent.last_heartbeat_at)}
                      </span>
                    </td>
                    <td className={s.mono}>{agent.capabilities?.profile || 'unknown'}</td>
                    <td className={s.mono}>{agent.capabilities?.runtime || 'unknown'}</td>
                    <td>
                      <CapabilityChip detail={agent.capabilities?.stdio} />
                    </td>
                    <td>
                      <CapabilityChip detail={agent.capabilities?.container_file} />
                    </td>
                    <td>
                      <span className={`${s.chip} ${agent.last_apply_status === 'success' ? s.chipGreen : s.chipRed}`}>
                        {agent.last_apply_status || '-'}
                      </span>
                    </td>
                  </tr>
                ))}
                {filtered.length === 0 && (
                  <tr>
                    <td colSpan={8} className={s.muted}>
                      {t('filebeat-k8s-app.agents.noAgents', 'No Agents.')}
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </section>

          <aside className={s.drawer}>
            {selected ? (
              <>
                <h2>{t('filebeat-k8s-app.agents.detailTitle', 'Agent detail')}</h2>
                <Summary label={t('filebeat-k8s-app.fields.agentId', 'Agent ID')} value={selected.id} />
                <Summary label={t('filebeat-k8s-app.fields.clusterId', 'Cluster ID')} value={selected.cluster_id} />
                <Summary label={t('filebeat-k8s-app.fields.nodeName', 'Node name')} value={selected.node_name} />
                <Summary label={t('filebeat-k8s-app.fields.currentChecksum', 'Current checksum')} value={shortChecksum(selected.current_config_checksum)} />
                <h3>{t('filebeat-k8s-app.agents.capabilities', 'Collection capability')}</h3>
                <CapabilityDetailBlock name={t('filebeat-k8s-app.fields.stdio', 'stdio')} detail={selected.capabilities?.stdio} />
                <CapabilityDetailBlock name={t('filebeat-k8s-app.fields.containerFile', 'container_file')} detail={selected.capabilities?.container_file} />
                <h3>{t('filebeat-k8s-app.fields.nodeLabels', 'Node labels')}</h3>
                <pre className={s.code} style={{ minHeight: 120, maxHeight: 220 }}>
                  {Object.entries(selected.node_labels ?? {})
                    .map(([key, value]) => `${key}=${value}`)
                    .join('\n') || t('filebeat-k8s-app.agents.noNodeLabels', 'No node_labels')}
                </pre>
                <h3>{t('filebeat-k8s-app.agents.recentApply', 'Recent apply')}</h3>
                <div className={s.message}>
                  <div>
                    <strong>{t('filebeat-k8s-app.fields.status', 'Status')}:</strong> {selected.last_apply_status || '-'}
                  </div>
                  <div>
                    <strong>{t('filebeat-k8s-app.fields.checksum', 'Checksum')}:</strong>{' '}
                    <span className={s.mono}>{selected.last_apply_checksum || '-'}</span>
                  </div>
                  <div>
                    <strong>{t('filebeat-k8s-app.fields.message', 'Message')}:</strong> {selected.last_apply_message || '-'}
                  </div>
                </div>
              </>
            ) : (
              <div className={s.muted}>{t('filebeat-k8s-app.agents.selectAgent', 'Select an Agent to view details.')}</div>
            )}
          </aside>
        </div>
      </div>
    </PluginPage>
  );
}

function CapabilityChip({ detail }: { detail?: CapabilityDetail }) {
  const s = useStyles2(getPageStyles);
  const status = detail?.status || 'unknown';
  return <span className={`${s.chip} ${capabilityClass(s, status)}`}>{status}</span>;
}

function CapabilityDetailBlock({ name, detail }: { name: string; detail?: CapabilityDetail }) {
  const s = useStyles2(getPageStyles);
  const status = detail?.status || 'unknown';
  return (
    <div className={s.message} style={{ marginBottom: 12 }}>
      <div>
        <strong>{name}:</strong> <span className={`${s.chip} ${capabilityClass(s, status)}`}>{status}</span>
      </div>
      <div>
        <strong>{t('filebeat-k8s-app.fields.detectedPath', 'Detected path')}:</strong>{' '}
        <span className={s.mono}>{detail?.detected_path || '-'}</span>
      </div>
      <div>
        <strong>{t('filebeat-k8s-app.fields.reason', 'Reason')}:</strong> {detail?.reason || '-'}
      </div>
    </div>
  );
}

function Summary({ label, value }: { label: string; value: string }) {
  const s = useStyles2(getPageStyles);
  return (
    <div className={s.card} style={{ marginBottom: 12 }}>
      <div className={s.metricLabel}>{label}</div>
      <strong className={s.mono}>{value || '-'}</strong>
    </div>
  );
}

function capabilityClass(s: ReturnType<typeof getPageStyles>, status: string): string {
  switch (status) {
    case 'ok':
      return s.chipGreen;
    case 'unsupported':
      return s.chipRed;
    case 'degraded':
      return s.chipOrange;
    default:
      return s.chipBlue;
  }
}

export default AgentsPage;
