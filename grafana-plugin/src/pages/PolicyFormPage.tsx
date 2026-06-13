import React, { ChangeEvent, useEffect, useMemo, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { PluginPage } from '@grafana/runtime';
import { Alert, Button, LinkButton, useStyles2 } from '@grafana/ui';
import { api } from '../api/client';
import { ClusterOptions, Policy } from '../types';
import { ROUTES } from '../constants';
import { prefixRoute } from '../utils/utils.routing';
import { customFieldsFromText, customFieldsToText, emptyPolicy, inputConfigFromYaml, inputConfigToYaml } from './utils';
import { getPageStyles } from './styles';

function PolicyFormPage() {
  const s = useStyles2(getPageStyles);
  const navigate = useNavigate();
  const { id } = useParams<{ id: string }>();
  const isEdit = Boolean(id);
  const [policy, setPolicy] = useState<Policy>(emptyPolicy());
  const [customFields, setCustomFields] = useState('');
  const [inputConfig, setInputConfig] = useState('');
  const [options, setOptions] = useState<ClusterOptions | undefined>();
  const [renderedConfig, setRenderedConfig] = useState('');
  const [renderedChecksum, setRenderedChecksum] = useState('');
  const [previewError, setPreviewError] = useState('');
  const [error, setError] = useState('');

  const parsedInputConfig = useMemo(() => {
    try {
      return { config: inputConfigFromYaml(inputConfig), error: '' };
    } catch (err) {
      return { config: {}, error: (err as Error).message };
    }
  }, [inputConfig]);

  useEffect(() => {
    api.clusterOptions().then(setOptions).catch(() => undefined);
    if (id) {
      api
        .getPolicy(id)
        .then((data) => {
          setPolicy(data);
          setCustomFields(customFieldsToText(data.custom_fields));
          setInputConfig(inputConfigToYaml(data.input_config));
          setRenderedConfig(data.rendered_config || '');
          setRenderedChecksum(data.rendered_checksum || '');
        })
        .catch((err) => setError(err.message));
    }
  }, [id]);

  useEffect(() => {
    if (parsedInputConfig.error) {
      return;
    }
    const next = { ...policy, custom_fields: customFieldsFromText(customFields), input_config: parsedInputConfig.config };
    const timer = window.setTimeout(() => {
      if (!next.name || !next.cluster_id) {
        return;
      }
      api
        .renderPreview(next)
        .then((preview) => {
          setPreviewError('');
          setRenderedConfig(preview.rendered_config);
          setRenderedChecksum(preview.rendered_checksum);
        })
        .catch((err) => setPreviewError(err.message));
    }, 450);
    return () => window.clearTimeout(timer);
  }, [customFields, parsedInputConfig, policy]);

  const update = (key: keyof Policy, value: string | number | boolean) => {
    setPolicy({ ...policy, [key]: value });
  };

  const onSave = async () => {
    if (parsedInputConfig.error) {
      return;
    }
    const payload = { ...policy, custom_fields: customFieldsFromText(customFields), input_config: parsedInputConfig.config };
    try {
      const saved = isEdit && id ? await api.updatePolicy(id, payload) : await api.createPolicy(payload);
      navigate(prefixRoute(`policies/${encodeURIComponent(saved.id)}`));
    } catch (err) {
      setError((err as Error).message);
    }
  };

  return (
    <PluginPage>
      <div className={s.page}>
        <div className={s.header}>
          <div>
            <div className={s.eyebrow}>Filebeat Ops / Policies / {isEdit ? '编辑策略' : '创建策略'}</div>
            <h1 className={s.title}>Policy Create / Edit</h1>
            <div className={s.subtitle}>使用 Kubernetes options 填充 scope，保存前预览 rendered_config。</div>
          </div>
          <div className={s.toolbar}>
            <LinkButton variant="secondary" href={prefixRoute(ROUTES.Policies)}>
              取消
            </LinkButton>
            <Button icon="save" onClick={onSave}>
              保存 policy
            </Button>
          </div>
        </div>

        {error && <Alert title="保存失败" severity="error">{error}</Alert>}
        {options?.degraded && <Alert title="Kubernetes options 降级" severity="warning">{options.message}</Alert>}

        <div className={s.split}>
          <section className={s.card}>
            <h2>基础信息</h2>
            <div className={s.formGrid}>
              <Field label="id" value={policy.id} onChange={(value) => update('id', value)} placeholder="payment-app" disabled={isEdit} />
              <Field label="name" value={policy.name} onChange={(value) => update('name', value)} placeholder="payment app" />
              <Field label="cluster_id" value={policy.cluster_id} onChange={(value) => update('cluster_id', value)} placeholder="dev" />
              <SelectField label="namespace" value={policy.namespace || ''} options={options?.namespaces ?? []} onChange={(value) => update('namespace', value)} />
              <SelectField
                label="controller_type"
                value={policy.controller_type || ''}
                options={['deployment', 'statefulset', 'daemonset', 'job', 'cronjob', 'pod', 'replicaset']}
                onChange={(value) => update('controller_type', value)}
              />
              <SelectField
                label="controller_name"
                value={policy.controller_name || ''}
                options={(options?.workloads ?? []).filter((workload) => !policy.namespace || workload.namespace === policy.namespace).map((workload) => workload.name)}
                onChange={(value) => update('controller_name', value)}
              />
              <SelectField
                label="container_name"
                value={policy.container_name || ''}
                options={(options?.containers ?? []).filter((container) => !policy.namespace || container.namespace === policy.namespace).map((container) => container.name)}
                onChange={(value) => update('container_name', value)}
              />
              <SelectField
                label="log_type"
                value={policy.log_type}
                options={['container_stdio', 'container_file', 'host_file']}
                onChange={(value) => update('log_type', value)}
              />
              <Field label="priority" value={String(policy.priority)} onChange={(value) => update('priority', Number(value) || 0)} />
              <div className={s.field}>
                <label>enabled</label>
                <div className={s.checkboxLine}>
                  <input type="checkbox" checked={policy.enabled} onChange={(event) => update('enabled', event.currentTarget.checked)} />
                  <span>{policy.enabled ? 'enabled' : 'disabled'}</span>
                </div>
              </div>
            </div>

            <h2>高级匹配</h2>
            <div className={s.formGrid}>
              <Field label="node_selector" value={policy.node_selector || ''} onChange={(value) => update('node_selector', value)} placeholder="nodepool=online,zone=hk" />
              <Field label="log_path" value={policy.log_path || ''} onChange={(value) => update('log_path', value)} placeholder="/app/logs/*.log 或 /var/log/messages" />
              <div className={`${s.field} ${s.fullSpan}`}>
                <label>custom_fields</label>
                <textarea className={s.input} rows={5} value={customFields} onChange={(event) => setCustomFields(event.target.value)} placeholder={'__project__=cloudnet\n__logstore__=payment'} />
              </div>
              <div className={`${s.field} ${s.fullSpan}`}>
                <label>input_config</label>
                <textarea
                  className={s.input}
                  rows={7}
                  value={inputConfig}
                  onChange={(event) => setInputConfig(event.target.value)}
                  placeholder={'scan_frequency: 10s\nignore_older: 72h\nexclude_files:\n  - "\\\\.gz$"'}
                />
                {parsedInputConfig.error && <div className={s.error}>{parsedInputConfig.error}</div>}
              </div>
            </div>
          </section>

          <section className={s.card}>
            <div className={s.header}>
              <div>
                <h2>YAML 预览</h2>
                <div className={`${s.muted} ${s.mono}`}>{renderedChecksum || 'rendered_checksum 待生成'}</div>
              </div>
              <span className={`${s.chip} ${s.chipBlue}`}>render-preview</span>
            </div>
            {previewError && <div className={s.error}>{previewError}</div>}
            <pre className={s.code}>{renderedConfig || '填写有效策略后自动生成 rendered_config。'}</pre>
          </section>
        </div>
      </div>
    </PluginPage>
  );
}

interface FieldProps {
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  disabled?: boolean;
}

function Field({ label, value, onChange, placeholder, disabled }: FieldProps) {
  const s = useStyles2(getPageStyles);
  return (
    <div className={s.field}>
      <label>{label}</label>
      <input className={s.input} value={value} placeholder={placeholder} disabled={disabled} onChange={(event: ChangeEvent<HTMLInputElement>) => onChange(event.target.value)} />
    </div>
  );
}

interface SelectFieldProps {
  label: string;
  value: string;
  options: string[];
  onChange: (value: string) => void;
}

function SelectField({ label, value, options, onChange }: SelectFieldProps) {
  const s = useStyles2(getPageStyles);
  const uniqueOptions = Array.from(new Set(options.filter(Boolean))).sort();
  return (
    <div className={s.field}>
      <label>{label}</label>
      <input className={s.input} value={value} list={`${label}-options`} onChange={(event) => onChange(event.target.value)} />
      <datalist id={`${label}-options`}>
        {uniqueOptions.map((option) => <option key={option} value={option} />)}
      </datalist>
    </div>
  );
}

export default PolicyFormPage;
