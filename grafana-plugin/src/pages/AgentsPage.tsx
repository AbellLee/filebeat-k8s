import React, { useEffect, useMemo, useState } from 'react';
import { PluginPage } from '@grafana/runtime';
import { Alert, Button, useStyles2 } from '@grafana/ui';
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
            <h1 className={s.title}>Agent 状态</h1>
            <div className={s.subtitle}>查看 sidecar heartbeat、apply-result 和节点日志采集能力。</div>
          </div>
          <div className={s.toolbar}>
            <select className={s.input} value={cluster} onChange={(event) => setCluster(event.target.value)}>
              <option value="">cluster: all</option>
              {clusters.map((item) => <option key={item} value={item}>{item}</option>)}
            </select>
            <Button variant="secondary" icon="sync" onClick={loadAgents}>
              刷新
            </Button>
          </div>
        </div>

        {error && <Alert title="加载失败" severity="error">{error}</Alert>}

        <div className={s.split}>
          <section className={s.card}>
            <table className={s.table}>
              <thead>
                <tr>
                  <th>agent_id</th>
                  <th>node_name</th>
                  <th>heartbeat</th>
                  <th>profile</th>
                  <th>runtime</th>
                  <th>stdio</th>
                  <th>container_file</th>
                  <th>last_apply_status</th>
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
                    <td><CapabilityChip detail={agent.capabilities?.stdio} /></td>
                    <td><CapabilityChip detail={agent.capabilities?.container_file} /></td>
                    <td>
                      <span className={`${s.chip} ${agent.last_apply_status === 'success' ? s.chipGreen : s.chipRed}`}>
                        {agent.last_apply_status || '-'}
                      </span>
                    </td>
                  </tr>
                ))}
                {filtered.length === 0 && (
                  <tr>
                    <td colSpan={8} className={s.muted}>暂无 Agent。</td>
                  </tr>
                )}
              </tbody>
            </table>
          </section>

          <aside className={s.drawer}>
            {selected ? (
              <>
                <h2>Agent detail</h2>
                <Summary label="agent_id" value={selected.id} />
                <Summary label="cluster_id" value={selected.cluster_id} />
                <Summary label="node_name" value={selected.node_name} />
                <Summary label="current checksum" value={shortChecksum(selected.current_config_checksum)} />
                <h3>采集能力</h3>
                <CapabilityDetailBlock name="stdio" detail={selected.capabilities?.stdio} />
                <CapabilityDetailBlock name="container_file" detail={selected.capabilities?.container_file} />
                <h3>node labels</h3>
                <pre className={s.code} style={{ minHeight: 120, maxHeight: 220 }}>
                  {Object.entries(selected.node_labels ?? {})
                    .map(([key, value]) => `${key}=${value}`)
                    .join('\n') || '暂无 node_labels'}
                </pre>
                <h3>最近 apply</h3>
                <div className={s.message}>
                  <div><strong>status:</strong> {selected.last_apply_status || '-'}</div>
                  <div><strong>checksum:</strong> <span className={s.mono}>{selected.last_apply_checksum || '-'}</span></div>
                  <div><strong>message:</strong> {selected.last_apply_message || '-'}</div>
                </div>
              </>
            ) : (
              <div className={s.muted}>选择一个 Agent 查看详情。</div>
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
      <div><strong>{name}:</strong> <span className={`${s.chip} ${capabilityClass(s, status)}`}>{status}</span></div>
      <div><strong>detected_path:</strong> <span className={s.mono}>{detail?.detected_path || '-'}</span></div>
      <div><strong>reason:</strong> {detail?.reason || '-'}</div>
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
