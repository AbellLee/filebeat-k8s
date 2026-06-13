import React, { useEffect, useMemo, useState } from 'react';
import { PluginPage } from '@grafana/runtime';
import { Alert, Button, LinkButton, useStyles2 } from '@grafana/ui';
import { api } from '../api/client';
import { Agent, Policy } from '../types';
import { ROUTES } from '../constants';
import { prefixRoute } from '../utils/utils.routing';
import { agentHealthy, policyScope, shortChecksum, timeAgo } from './utils';
import { getPageStyles } from './styles';

function OverviewPage() {
  const s = useStyles2(getPageStyles);
  const [policies, setPolicies] = useState<Policy[]>([]);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [error, setError] = useState('');

  useEffect(() => {
    Promise.all([api.listPolicies(), api.listAgents()])
      .then(([policyData, agentData]) => {
        setPolicies(policyData);
        setAgents(agentData);
        setError('');
      })
      .catch((err) => setError(err.message));
  }, []);

  const enabledPolicies = policies.filter((policy) => policy.enabled);
  const healthyAgents = agents.filter(agentHealthy);
  const failedAgents = agents.filter((agent) => agent.last_apply_status && agent.last_apply_status !== 'success');
  const recentPolicies = useMemo(() => policies.slice(0, 5), [policies]);

  return (
    <PluginPage>
      <div className={s.page}>
        <div className={s.header}>
          <div>
            <div className={s.eyebrow}>Filebeat Ops / Overview</div>
            <h1 className={s.title}>日志采集总览</h1>
            <div className={s.subtitle}>查看策略覆盖、Agent 健康和最近 apply 结果。</div>
          </div>
          <div className={s.toolbar}>
            <LinkButton icon="plus" href={prefixRoute(ROUTES.PolicyNew)}>
              新建策略
            </LinkButton>
            <LinkButton variant="secondary" href={prefixRoute(ROUTES.Agents)}>
              查看 Agents
            </LinkButton>
          </div>
        </div>

        {error && <Alert title="加载失败" severity="error">{error}</Alert>}

        <div className={s.grid4}>
          <Metric label="策略总数 policies" value={policies.length} hint="当前 control-server 策略" />
          <Metric label="启用策略 enabled" value={enabledPolicies.length} hint={`${policies.length ? Math.round((enabledPolicies.length / policies.length) * 100) : 0}% 生效中`} />
          <Metric label="Agent 健康" value={`${healthyAgents.length}/${agents.length}`} hint="5 分钟内 heartbeat" />
          <Metric label="最近 apply 失败" value={failedAgents.length} hint="需要排查" danger={failedAgents.length > 0} />
        </div>

        <div className={s.grid2}>
          <section className={s.card}>
            <h2>最近策略</h2>
            <table className={s.table}>
              <thead>
                <tr>
                  <th>name</th>
                  <th>scope</th>
                  <th>log_type</th>
                  <th>revision</th>
                  <th>checksum</th>
                </tr>
              </thead>
              <tbody>
                {recentPolicies.map((policy) => (
                  <tr key={policy.id}>
                    <td>
                      <strong>{policy.name}</strong>
                      <div className={s.muted}>{policy.id}</div>
                    </td>
                    <td className={s.mono}>{policyScope(policy)}</td>
                    <td><span className={`${s.chip} ${s.chipBlue}`}>{policy.log_type}</span></td>
                    <td>{policy.current_revision}</td>
                    <td className={s.mono}>{shortChecksum(policy.rendered_checksum)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </section>

          <section className={s.card}>
            <h2>最近 apply 失败</h2>
            <table className={s.table}>
              <thead>
                <tr>
                  <th>agent_id</th>
                  <th>checksum</th>
                  <th>status</th>
                  <th>message</th>
                </tr>
              </thead>
              <tbody>
                {failedAgents.map((agent) => (
                  <tr key={agent.id}>
                    <td className={s.mono}>{agent.id}</td>
                    <td className={s.mono}>{shortChecksum(agent.last_apply_checksum)}</td>
                    <td><span className={`${s.chip} ${s.chipRed}`}>{agent.last_apply_status}</span></td>
                    <td>{agent.last_apply_message || '-'}</td>
                  </tr>
                ))}
                {failedAgents.length === 0 && (
                  <tr>
                    <td colSpan={4} className={s.muted}>暂无失败 apply 结果。</td>
                  </tr>
                )}
              </tbody>
            </table>
          </section>
        </div>

        <section className={s.card}>
          <h2>关键闭环</h2>
          <div className={s.toolbar}>
            <span className={`${s.chip} ${s.chipBlue}`}>创建策略</span>
            <span>→</span>
            <span className={`${s.chip} ${s.chipBlue}`}>预览 YAML</span>
            <span>→</span>
            <span className={`${s.chip} ${s.chipBlue}`}>保存</span>
            <span>→</span>
            <span className={`${s.chip} ${s.chipBlue}`}>查看 revision</span>
            <span>→</span>
            <span className={`${s.chip} ${s.chipBlue}`}>回滚</span>
            <span>→</span>
            <span className={`${s.chip} ${s.chipBlue}`}>检查 Agent apply</span>
            <Button variant="secondary" onClick={() => window.location.assign(prefixRoute(ROUTES.PolicyNew))}>
              开始
            </Button>
          </div>
          <div className={s.subtitle}>最近 heartbeat：{agents[0] ? `${agents[0].id} ${timeAgo(agents[0].last_heartbeat_at)}` : '-'}</div>
        </section>
      </div>
    </PluginPage>
  );
}

interface MetricProps {
  label: string;
  value: React.ReactNode;
  hint: string;
  danger?: boolean;
}

function Metric({ label, value, hint, danger }: MetricProps) {
  const s = useStyles2(getPageStyles);
  return (
    <section className={s.card}>
      <div className={s.metricLabel}>{label}</div>
      <div className={s.metricValue}>{value}</div>
      <div className={danger ? s.danger : s.muted}>{hint}</div>
    </section>
  );
}

export default OverviewPage;
