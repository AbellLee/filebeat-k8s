import React, { useEffect, useState } from 'react';
import { useParams } from 'react-router-dom';
import { PluginPage } from '@grafana/runtime';
import { Alert, Button, LinkButton, Modal, useStyles2 } from '@grafana/ui';
import { api } from '../api/client';
import { Policy, PolicyRevision } from '../types';
import { ROUTES } from '../constants';
import { prefixRoute } from '../utils/utils.routing';
import { formatDate, policyScope, shortChecksum } from './utils';
import { getPageStyles } from './styles';

function PolicyDetailPage() {
  const s = useStyles2(getPageStyles);
  const { id } = useParams<{ id: string }>();
  const [policy, setPolicy] = useState<Policy | undefined>();
  const [revisions, setRevisions] = useState<PolicyRevision[]>([]);
  const [rollbackTarget, setRollbackTarget] = useState<PolicyRevision | undefined>();
  const [error, setError] = useState('');

  const load = () => {
    if (!id) {
      return;
    }
    Promise.all([api.getPolicy(id), api.listRevisions(id)])
      .then(([policyData, revisionData]) => {
        setPolicy(policyData);
        setRevisions(revisionData);
        setError('');
      })
      .catch((err) => setError(err.message));
  };

  useEffect(load, [id]);

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
        {error ? <Alert title="加载失败" severity="error">{error}</Alert> : <div className={s.muted}>加载 policy...</div>}
      </PluginPage>
    );
  }

  return (
    <PluginPage>
      <div className={s.page}>
        <div className={s.header}>
          <div>
            <div className={s.eyebrow}>Filebeat Ops / Policies / {policy.id}</div>
            <h1 className={s.title}>Policy Detail</h1>
            <div className={s.subtitle}>查看当前 spec、rendered_config 和 revision 历史，支持回滚。</div>
          </div>
          <div className={s.toolbar}>
            <LinkButton variant="secondary" href={prefixRoute(ROUTES.Policies)}>
              返回列表
            </LinkButton>
            <LinkButton icon="edit" href={prefixRoute(`policies/${encodeURIComponent(policy.id)}/edit`)}>
              编辑
            </LinkButton>
          </div>
        </div>

        {error && <Alert title="操作失败" severity="error">{error}</Alert>}

        <div className={s.grid4}>
          <Summary label="policy_id" value={policy.id} />
          <Summary label="scope" value={policyScope(policy)} />
          <Summary label="current_revision" value={String(policy.current_revision)} />
          <Summary label="rendered_checksum" value={shortChecksum(policy.rendered_checksum)} />
        </div>

        <div className={s.grid2}>
          <section className={s.card}>
            <h2>rendered_config</h2>
            <pre className={s.code}>{policy.rendered_config || '当前 policy 尚无 rendered_config。'}</pre>
          </section>

          <section className={s.card}>
            <h2>Revision history</h2>
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
                        <span className={`${s.chip} ${s.chipGreen}`}>current</span>
                      ) : (
                        <Button size="sm" variant="secondary" onClick={() => setRollbackTarget(revision)}>
                          rollback
                        </Button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            <div className={s.message}>POST /api/v1/policies/:id/rollback 会生成新的 revision，并让 Agent 在下次拉取时看到新 checksum。</div>
          </section>
        </div>
      </div>

      {rollbackTarget && (
        <Modal title={`确认回滚 revision ${rollbackTarget.revision}？`} isOpen={Boolean(rollbackTarget)} onDismiss={() => setRollbackTarget(undefined)}>
          <p>系统会复制目标 revision 的 rendered_config 为新的 revision。</p>
          <div className={s.card}>
            <div className={s.muted}>target checksum</div>
            <strong className={s.mono}>{rollbackTarget.checksum}</strong>
          </div>
          <div className={s.toolbar} style={{ justifyContent: 'flex-end', marginTop: 16 }}>
            <Button variant="secondary" onClick={() => setRollbackTarget(undefined)}>
              取消
            </Button>
            <Button onClick={rollback}>
              确认回滚
            </Button>
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
      <div className={`${s.metricValue} ${s.mono}`} style={{ fontSize: 15 }}>{value || '-'}</div>
    </section>
  );
}

export default PolicyDetailPage;
