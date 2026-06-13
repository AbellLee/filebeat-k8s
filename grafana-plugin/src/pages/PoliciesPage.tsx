import React, { useEffect, useMemo, useState } from 'react';
import { PluginPage } from '@grafana/runtime';
import { Alert, Button, LinkButton, useStyles2 } from '@grafana/ui';
import { api } from '../api/client';
import { Policy } from '../types';
import { ROUTES } from '../constants';
import { prefixRoute } from '../utils/utils.routing';
import { formatDate, policyScope, shortChecksum } from './utils';
import { getPageStyles } from './styles';

type Filters = {
  cluster: string;
  namespace: string;
  logType: string;
  enabled: string;
  query: string;
};

function PoliciesPage() {
  const s = useStyles2(getPageStyles);
  const [policies, setPolicies] = useState<Policy[]>([]);
  const [error, setError] = useState('');
  const [filters, setFilters] = useState<Filters>({ cluster: '', namespace: '', logType: '', enabled: '', query: '' });

  const loadPolicies = () => {
    api
      .listPolicies()
      .then((data) => {
        setPolicies(data);
        setError('');
      })
      .catch((err) => setError(err.message));
  };

  useEffect(loadPolicies, []);

  const filtered = useMemo(() => {
    const query = filters.query.toLowerCase();
    return policies.filter((policy) => {
      if (filters.cluster && policy.cluster_id !== filters.cluster) {
        return false;
      }
      if (filters.namespace && policy.namespace !== filters.namespace) {
        return false;
      }
      if (filters.logType && policy.log_type !== filters.logType) {
        return false;
      }
      if (filters.enabled && String(policy.enabled) !== filters.enabled) {
        return false;
      }
      if (query) {
        return `${policy.id} ${policy.name} ${policyScope(policy)}`.toLowerCase().includes(query);
      }
      return true;
    });
  }, [filters, policies]);

  const clusters = unique(policies.map((policy) => policy.cluster_id));
  const namespaces = unique(policies.map((policy) => policy.namespace || '').filter(Boolean));

  const togglePolicy = async (policy: Policy) => {
    try {
      await api.updatePolicy(policy.id, { ...policy, enabled: !policy.enabled });
      loadPolicies();
    } catch (err) {
      setError((err as Error).message);
    }
  };

  const deletePolicy = async (policy: Policy) => {
    if (!window.confirm(`确认删除 policy ${policy.id}？`)) {
      return;
    }
    try {
      await api.deletePolicy(policy.id);
      loadPolicies();
    } catch (err) {
      setError((err as Error).message);
    }
  };

  return (
    <PluginPage>
      <div className={s.page}>
        <div className={s.header}>
          <div>
            <div className={s.eyebrow}>Filebeat Ops / Policies</div>
            <h1 className={s.title}>策略 Policies</h1>
            <div className={s.subtitle}>管理 Filebeat 采集策略，支持启停、编辑、删除和 revision 查看。</div>
          </div>
          <LinkButton icon="plus" href={prefixRoute(ROUTES.PolicyNew)}>
            新建 policy
          </LinkButton>
        </div>

        {error && <Alert title="操作失败" severity="error">{error}</Alert>}

        <section className={s.card}>
          <div className={s.toolbar}>
            <select className={s.input} value={filters.cluster} onChange={(event) => setFilters({ ...filters, cluster: event.target.value })}>
              <option value="">cluster: all</option>
              {clusters.map((cluster) => <option key={cluster} value={cluster}>{cluster}</option>)}
            </select>
            <select className={s.input} value={filters.namespace} onChange={(event) => setFilters({ ...filters, namespace: event.target.value })}>
              <option value="">namespace: all</option>
              {namespaces.map((namespace) => <option key={namespace} value={namespace}>{namespace}</option>)}
            </select>
            <select className={s.input} value={filters.logType} onChange={(event) => setFilters({ ...filters, logType: event.target.value })}>
              <option value="">log_type: all</option>
              <option value="container_stdio">container_stdio</option>
              <option value="container_file">container_file</option>
              <option value="host_file">host_file</option>
            </select>
            <select className={s.input} value={filters.enabled} onChange={(event) => setFilters({ ...filters, enabled: event.target.value })}>
              <option value="">enabled: all</option>
              <option value="true">enabled</option>
              <option value="false">disabled</option>
            </select>
            <input className={s.input} placeholder="搜索 name / id / scope" value={filters.query} onChange={(event) => setFilters({ ...filters, query: event.target.value })} />
            <Button variant="secondary" onClick={() => setFilters({ cluster: '', namespace: '', logType: '', enabled: '', query: '' })}>
              清空
            </Button>
          </div>
        </section>

        <section className={s.card}>
          <table className={s.table}>
            <thead>
              <tr>
                <th>name</th>
                <th>scope</th>
                <th>log_type</th>
                <th>enabled</th>
                <th>revision</th>
                <th>checksum</th>
                <th>updated_at</th>
                <th>actions</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((policy) => (
                <tr key={policy.id}>
                  <td>
                    <strong>{policy.name}</strong>
                    <div className={`${s.muted} ${s.mono}`}>{policy.id}</div>
                  </td>
                  <td className={s.mono}>{policyScope(policy)}</td>
                  <td><span className={`${s.chip} ${policy.log_type === 'host_file' ? s.chipOrange : s.chipBlue}`}>{policy.log_type}</span></td>
                  <td><span className={`${s.chip} ${policy.enabled ? s.chipGreen : ''}`}>{policy.enabled ? 'enabled' : 'disabled'}</span></td>
                  <td>{policy.current_revision}</td>
                  <td className={s.mono}>{shortChecksum(policy.rendered_checksum)}</td>
                  <td>{formatDate(policy.updated_at)}</td>
                  <td>
                    <div className={s.rowActions}>
                      <LinkButton size="sm" variant="secondary" href={prefixRoute(`policies/${encodeURIComponent(policy.id)}`)}>
                        详情
                      </LinkButton>
                      <LinkButton size="sm" variant="secondary" href={prefixRoute(`policies/${encodeURIComponent(policy.id)}/edit`)}>
                        编辑
                      </LinkButton>
                      <Button size="sm" variant="secondary" onClick={() => togglePolicy(policy)}>
                        {policy.enabled ? '停用' : '启用'}
                      </Button>
                      <Button size="sm" variant="destructive" onClick={() => deletePolicy(policy)}>
                        删除
                      </Button>
                    </div>
                  </td>
                </tr>
              ))}
              {filtered.length === 0 && (
                <tr>
                  <td colSpan={8} className={s.muted}>没有匹配的 policy。</td>
                </tr>
              )}
            </tbody>
          </table>
        </section>
      </div>
    </PluginPage>
  );
}

function unique(values: string[]): string[] {
  return Array.from(new Set(values)).sort();
}

export default PoliciesPage;
