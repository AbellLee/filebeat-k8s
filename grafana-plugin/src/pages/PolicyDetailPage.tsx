import React, { useEffect, useMemo, useState } from 'react';
import { useParams } from 'react-router-dom';
import { PluginPage } from '@grafana/runtime';
import { Alert, Button, LinkButton, Modal, useStyles2 } from '@grafana/ui';
import { t } from '@grafana/i18n';
import { api } from '../api/client';
import { Agent, Policy, PolicyRevision } from '../types';
import { ROUTES } from '../constants';
import { prefixRoute } from '../utils/utils.routing';
import { formatDate, policyScope, shortChecksum } from './utils';
import { getPageStyles } from './styles';

function PolicyDetailPage() {
  const s = useStyles2(getPageStyles);
  const { id } = useParams<{ id: string }>();
  const [policy, setPolicy] = useState<Policy | undefined>();
  const [revisions, setRevisions] = useState<PolicyRevision[]>([]);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [rollbackTarget, setRollbackTarget] = useState<PolicyRevision | undefined>();
  const [error, setError] = useState('');

  const load = () => {
    if (!id) {
      return;
    }
    Promise.all([api.getPolicy(id), api.listRevisions(id), api.listAgents()])
      .then(([policyData, revisionData, agentData]) => {
        setPolicy(policyData);
        setRevisions(revisionData);
        setAgents(agentData);
        setError('');
      })
      .catch((err) => setError(err.message));
  };

  useEffect(load, [id]);

  const containerFileSummary = useMemo(() => {
    if (!policy || policy.log_type !== 'container_file') {
      return undefined;
    }
    const matching = agents.filter((agent) => agent.cluster_id === policy.cluster_id && nodeSelectorMatches(policy.node_selector || '', agent.node_labels ?? {}));
    const unsupported = matching.filter((agent) => agent.capabilities?.container_file?.status !== 'ok');
    return { matching, unsupported };
  }, [agents, policy]);

  const rollback = async () => {
    if (!id || !rollbackTarget) {
      return;
    }
    try {
      await api.rollbackPolicy(id, rollbackTarget.revision);
      setRollbackTarget(undefined);
      load();
    } catch (err) {
      setError((err as Error).message);
    }
  };

  if (!policy) {
    return (
      <PluginPage>
        {error ? (
          <Alert title={t('filebeat-k8s-app.common.loadFailed', 'Load failed')} severity="error">
            {error}
          </Alert>
        ) : (
          <div className={s.muted}>{t('filebeat-k8s-app.common.loadingPolicy', 'Loading policy...')}</div>
        )}
      </PluginPage>
    );
  }

  return (
    <PluginPage>
      <div className={s.page}>
        <div className={s.header}>
          <div>
            <div className={s.eyebrow}>Filebeat Ops / Policies / {policy.id}</div>
            <h1 className={s.title}>{t('filebeat-k8s-app.policyDetail.title', 'Policy detail')}</h1>
            <div className={s.subtitle}>
              {t('filebeat-k8s-app.policyDetail.subtitle', 'Review the current spec, rendered_config, and revision history, with rollback support.')}
            </div>
          </div>
          <div className={s.toolbar}>
            <LinkButton variant="secondary" href={prefixRoute(ROUTES.Policies)}>
              {t('filebeat-k8s-app.common.backToList', 'Back to list')}
            </LinkButton>
            <LinkButton icon="edit" href={prefixRoute(`policies/${encodeURIComponent(policy.id)}/edit`)}>
              {t('filebeat-k8s-app.common.edit', 'Edit')}
            </LinkButton>
          </div>
        </div>

        {error && (
          <Alert title={t('filebeat-k8s-app.common.operationFailed', 'Operation failed')} severity="error">
            {error}
          </Alert>
        )}
        {containerFileSummary && containerFileSummary.matching.length > 0 && containerFileSummary.unsupported.length > 0 && (
          <Alert
            title={t('filebeat-k8s-app.policyDetail.containerFileCapabilityWarning', 'container_file node capability is insufficient')}
            severity="warning"
          >
            {t(
              'filebeat-k8s-app.policyDetail.containerFileCapabilityMessage',
              '{{unsupported}}/{{total}} matching Agents do not currently support in-container file collection. Check the Agents page for reasons.',
              {
                unsupported: containerFileSummary.unsupported.length,
                total: containerFileSummary.matching.length,
              }
            )}
          </Alert>
        )}

        <div className={s.grid4}>
          <Summary label="policy_id" value={policy.id} />
          <Summary label="scope" value={policyScope(policy)} />
          <Summary label="current_revision" value={String(policy.current_revision)} />
          <Summary label="rendered_checksum" value={shortChecksum(policy.rendered_checksum)} />
        </div>

        <div className={s.grid2}>
          <section className={s.card}>
            <h2>rendered_config</h2>
            <pre className={s.code}>{policy.rendered_config || t('filebeat-k8s-app.policyDetail.noRenderedConfig', 'This policy has no rendered_config yet.')}</pre>
          </section>

          <section className={s.card}>
            <h2>{t('filebeat-k8s-app.policyDetail.revisionHistory', 'Revision history')}</h2>
            <table className={s.table}>
              <thead>
                <tr>
                  <th>revision</th>
                  <th>checksum</th>
                  <th>created_by</th>
                  <th>created_at</th>
                  <th>action</th>
                </tr>
              </thead>
              <tbody>
                {revisions.map((revision) => (
                  <tr key={revision.revision}>
                    <td>{revision.revision}</td>
                    <td className={s.mono}>{shortChecksum(revision.checksum)}</td>
                    <td>{revision.created_by}</td>
                    <td>{formatDate(revision.created_at)}</td>
                    <td>
                      {revision.revision === policy.current_revision ? (
                        <span className={`${s.chip} ${s.chipGreen}`}>{t('filebeat-k8s-app.common.current', 'current')}</span>
                      ) : (
                        <Button size="sm" variant="secondary" onClick={() => setRollbackTarget(revision)}>
                          {t('filebeat-k8s-app.common.rollback', 'rollback')}
                        </Button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            <div className={s.message}>
              {t(
                'filebeat-k8s-app.policyDetail.rollbackInfo',
                'POST /api/v1/policies/:id/rollback creates a new revision and lets Agents see the new checksum on their next pull.'
              )}
            </div>
          </section>
        </div>
      </div>

      {rollbackTarget && (
        <Modal
          title={t('filebeat-k8s-app.policyDetail.confirmRollbackTitle', 'Confirm rollback to revision {{revision}}?', {
            revision: rollbackTarget.revision,
          })}
          isOpen={Boolean(rollbackTarget)}
          onDismiss={() => setRollbackTarget(undefined)}
        >
          <p>
            {t(
              'filebeat-k8s-app.policyDetail.confirmRollbackBody',
              'The system will copy the target revision rendered_config into a new revision.'
            )}
          </p>
          <div className={s.card}>
            <div className={s.muted}>target checksum</div>
            <strong className={s.mono}>{rollbackTarget.checksum}</strong>
          </div>
          <div className={s.toolbar} style={{ justifyContent: 'flex-end', marginTop: 16 }}>
            <Button variant="secondary" onClick={() => setRollbackTarget(undefined)}>
              {t('filebeat-k8s-app.common.cancel', 'Cancel')}
            </Button>
            <Button onClick={rollback}>{t('filebeat-k8s-app.policyDetail.confirmRollback', 'Confirm rollback')}</Button>
          </div>
        </Modal>
      )}
    </PluginPage>
  );
}

function Summary({ label, value }: { label: string; value: string }) {
  const s = useStyles2(getPageStyles);
  return (
    <section className={s.card}>
      <div className={s.metricLabel}>{label}</div>
      <div className={`${s.metricValue} ${s.mono}`} style={{ fontSize: 15 }}>
        {value || '-'}
      </div>
    </section>
  );
}

function nodeSelectorMatches(selector: string, labels: Record<string, string>): boolean {
  const trimmed = selector.trim();
  if (!trimmed) {
    return true;
  }
  for (const segment of trimmed.split(',')) {
    const [rawKey, rawValue] = segment.split('=');
    const key = rawKey?.trim();
    const value = rawValue?.trim();
    if (!key || !value || labels[key] !== value) {
      return false;
    }
  }
  return true;
}

export default PolicyDetailPage;
