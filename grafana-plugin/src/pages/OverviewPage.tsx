import React, { useEffect, useMemo, useState } from 'react';
import { PluginPage } from '@grafana/runtime';
import { Alert, Button, LinkButton, useStyles2 } from '@grafana/ui';
import { t } from '@grafana/i18n';
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
  const enabledPercent = policies.length ? Math.round((enabledPolicies.length / policies.length) * 100) : 0;
  const latestHeartbeat = agents[0] ? `${agents[0].id} ${timeAgo(agents[0].last_heartbeat_at)}` : '-';

  return (
    <PluginPage>
      <div className={s.page}>
        <div className={s.header}>
          <div>
            <div className={s.eyebrow}>Filebeat Ops / Overview</div>
            <h1 className={s.title}>{t('filebeat-k8s-app.overview.title', 'Log collection overview')}</h1>
            <div className={s.subtitle}>
              {t('filebeat-k8s-app.overview.subtitle', 'Review policy coverage, agent health, and recent apply results.')}
            </div>
          </div>
          <div className={s.toolbar}>
            <LinkButton icon="plus" href={prefixRoute(ROUTES.PolicyNew)}>
              {t('filebeat-k8s-app.overview.newPolicy', 'New policy')}
            </LinkButton>
            <LinkButton variant="secondary" href={prefixRoute(ROUTES.Agents)}>
              {t('filebeat-k8s-app.overview.viewAgents', 'View Agents')}
            </LinkButton>
          </div>
        </div>

        {error && (
          <Alert title={t('filebeat-k8s-app.common.loadFailed', 'Load failed')} severity="error">
            {error}
          </Alert>
        )}

        <div className={s.grid4}>
          <Metric
            label={t('filebeat-k8s-app.overview.policiesTotal', 'Total policies')}
            value={policies.length}
            hint={t('filebeat-k8s-app.overview.policiesTotalHint', 'Policies currently stored in control-server')}
          />
          <Metric
            label={t('filebeat-k8s-app.overview.enabledPolicies', 'Enabled policies')}
            value={enabledPolicies.length}
            hint={t('filebeat-k8s-app.overview.enabledPoliciesHint', '{{percent}}% active', { percent: enabledPercent })}
          />
          <Metric
            label={t('filebeat-k8s-app.overview.agentHealth', 'Agent health')}
            value={`${healthyAgents.length}/${agents.length}`}
            hint={t('filebeat-k8s-app.overview.agentHealthHint', 'Heartbeat within 5 minutes')}
          />
          <Metric
            label={t('filebeat-k8s-app.overview.recentApplyFailures', 'Recent apply failures')}
            value={failedAgents.length}
            hint={t('filebeat-k8s-app.overview.recentApplyFailuresHint', 'Needs investigation')}
            danger={failedAgents.length > 0}
          />
        </div>

        <div className={s.grid2}>
          <section className={s.card}>
            <h2>{t('filebeat-k8s-app.overview.recentPolicies', 'Recent policies')}</h2>
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
                    <td>
                      <span className={`${s.chip} ${s.chipBlue}`}>{policy.log_type}</span>
                    </td>
                    <td>{policy.current_revision}</td>
                    <td className={s.mono}>{shortChecksum(policy.rendered_checksum)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </section>

          <section className={s.card}>
            <h2>{t('filebeat-k8s-app.overview.recentApplyFailures', 'Recent apply failures')}</h2>
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
                    <td>
                      <span className={`${s.chip} ${s.chipRed}`}>{agent.last_apply_status}</span>
                    </td>
                    <td>{agent.last_apply_message || '-'}</td>
                  </tr>
                ))}
                {failedAgents.length === 0 && (
                  <tr>
                    <td colSpan={4} className={s.muted}>
                      {t('filebeat-k8s-app.overview.noApplyFailures', 'No failed apply results.')}
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </section>
        </div>

        <section className={s.card}>
          <h2>{t('filebeat-k8s-app.overview.keyFlow', 'Key workflow')}</h2>
          <div className={s.toolbar}>
            <span className={`${s.chip} ${s.chipBlue}`}>{t('filebeat-k8s-app.overview.createPolicy', 'Create policy')}</span>
            <span>-&gt;</span>
            <span className={`${s.chip} ${s.chipBlue}`}>{t('filebeat-k8s-app.overview.previewYaml', 'Preview YAML')}</span>
            <span>-&gt;</span>
            <span className={`${s.chip} ${s.chipBlue}`}>{t('filebeat-k8s-app.overview.save', 'Save')}</span>
            <span>-&gt;</span>
            <span className={`${s.chip} ${s.chipBlue}`}>{t('filebeat-k8s-app.overview.viewRevision', 'View revision')}</span>
            <span>-&gt;</span>
            <span className={`${s.chip} ${s.chipBlue}`}>{t('filebeat-k8s-app.common.rollback', 'rollback')}</span>
            <span>-&gt;</span>
            <span className={`${s.chip} ${s.chipBlue}`}>{t('filebeat-k8s-app.overview.checkAgentApply', 'Check Agent apply')}</span>
            <Button variant="secondary" onClick={() => window.location.assign(prefixRoute(ROUTES.PolicyNew))}>
              {t('filebeat-k8s-app.overview.start', 'Start')}
            </Button>
          </div>
          <div className={s.subtitle}>
            {t('filebeat-k8s-app.overview.recentHeartbeat', 'Latest heartbeat: {{value}}', { value: latestHeartbeat })}
          </div>
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
