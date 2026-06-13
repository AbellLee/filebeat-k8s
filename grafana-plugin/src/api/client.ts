import { lastValueFrom } from 'rxjs';
import { getBackendSrv } from '@grafana/runtime';
import pluginJson from '../plugin.json';
import { Agent, ClusterOptions, Policy, PolicyRevision, RenderPreviewResponse } from '../types';

const resourceBase = `/api/plugins/${pluginJson.id}/resources`;

async function request<T>(path: string, options: { method?: string; data?: unknown } = {}): Promise<T> {
  try {
    const response = getBackendSrv().fetch<T>({
      url: `${resourceBase}/${path}`,
      method: options.method ?? 'GET',
      data: options.data,
    });
    const result = await lastValueFrom(response);
    return result.data;
  } catch (error) {
    throw normalizeError(error);
  }
}

function asArray<T>(value: T[] | null | undefined): T[] {
  return Array.isArray(value) ? value : [];
}

function normalizeError(error: unknown): Error {
  const raw = error as { data?: { error?: string; message?: string; details?: string }; status?: number; message?: string };
  const message = raw?.data?.error || raw?.data?.message || raw?.message || '请求失败';
  const details = raw?.data?.details ? ` ${raw.data.details}` : '';
  return new Error(`${message}${details}`);
}

export const api = {
  readyz: () => request<{ status: string }>('readyz'),
  listPolicies: () => request<Policy[] | null>('policies').then(asArray),
  getPolicy: (id: string) => request<Policy>(`policies/${encodeURIComponent(id)}`),
  createPolicy: (policy: Policy) => request<Policy>('policies', { method: 'POST', data: policy }),
  updatePolicy: (id: string, policy: Policy) =>
    request<Policy>(`policies/${encodeURIComponent(id)}`, { method: 'PUT', data: policy }),
  deletePolicy: (id: string) => request<{ status: string }>(`policies/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  renderPreview: (policy: Policy) =>
    request<RenderPreviewResponse>('policies/render-preview', { method: 'POST', data: policy }),
  listRevisions: (id: string) => request<PolicyRevision[] | null>(`policies/${encodeURIComponent(id)}/revisions`).then(asArray),
  rollbackPolicy: (id: string, revision: number) =>
    request<PolicyRevision>(`policies/${encodeURIComponent(id)}/rollback`, { method: 'POST', data: { revision } }),
  listAgents: () => request<Agent[] | null>('agents').then(asArray),
  getAgent: (id: string) => request<Agent>(`agents/${encodeURIComponent(id)}`),
  clusterOptions: () => request<ClusterOptions>('cluster/options'),
};
