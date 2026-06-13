import React, { useEffect, useMemo, useState } from 'react';
import { PluginPage } from '@grafana/runtime';
import { Alert, Button, useStyles2 } from '@grafana/ui';
import { api } from '../api/client';
import { Agent } from '../types';
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
            <div className={s.subtitle}>查看 sidecar 注册、heartbeat、checksum 和 apply-result。</div>
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
                  <th>current_config_checksum</th>
                  <th>last_apply_status</th>
                  <th>message</th>
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
                    <td className={s.mono}>{shortChecksum(agent.current_config_checksum)}</td>
                    <td>
                      <span className={`${s.chip} ${agent.last_apply_status === 'success' ? s.chipGreen : s.chipRed}`}>
                        {agent.last_apply_status || '-'}
                      </span>
                    </td>
                    <td>{agent.last_apply_message || '-'}</td>
                  </tr>
                ))}
                {filtered.length === 0 && (
                  <tr>
                    <td colSpan={6} className={s.muted}>暂无 Agent。</td>
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

function Summary({ label, value }: { label: string; value: string }) {
  const s = useStyles2(getPageStyles);
  return (
    <div className={s.card} style={{ marginBottom: 12 }}>
      <div className={s.metricLabel}>{label}</div>
      <strong className={s.mono}>{value || '-'}</strong>
    </div>
  );
}

export default AgentsPage;
