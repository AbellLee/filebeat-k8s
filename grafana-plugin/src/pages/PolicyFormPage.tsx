import React, { ChangeEvent, useEffect, useMemo, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { PluginPage } from '@grafana/runtime';
import { Alert, Button, Combobox, ComboboxOption, LinkButton, useStyles2 } from '@grafana/ui';
import { t } from '@grafana/i18n';
import { api } from '../api/client';
import { ClusterOptions, LogType, Policy } from '../types';
import { ROUTES } from '../constants';
import { prefixRoute } from '../utils/utils.routing';
import { customFieldsFromText, customFieldsToText, emptyPolicy, inputConfigFromYaml, inputConfigToYaml } from './utils';
import { getPageStyles } from './styles';

const controllerTypeOptions = ['deployment', 'statefulset', 'daemonset', 'job', 'cronjob', 'pod', 'replicaset'];
const logTypeOptions: LogType[] = ['container_stdio', 'container_file', 'host_file'];

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

  const isHostFile = policy.log_type === 'host_file';
  const selectedNamespace = policy.namespace || '';
  const selectedControllerType = policy.controller_type || '';
  const selectedControllerName = policy.controller_name || '';

  const namespaceOptions = useMemo(() => options?.namespaces ?? [], [options]);

  const availableControllerTypes = useMemo(() => {
    if (!selectedNamespace) {
      return controllerTypeOptions;
    }
    const available = new Set<string>();
    for (const workload of options?.workloads ?? []) {
      if (workload.namespace === selectedNamespace) {
        available.add(workload.controller_type);
      }
    }
    if ((options?.pods ?? []).some((pod) => pod.namespace === selectedNamespace)) {
      available.add('pod');
    }
    return available.size > 0 ? controllerTypeOptions.filter((type) => available.has(type)) : controllerTypeOptions;
  }, [options, selectedNamespace]);

  const controllerNameOptions = useMemo(() => {
    if (!selectedNamespace || !selectedControllerType) {
      return [];
    }
    if (selectedControllerType === 'pod') {
      return (options?.pods ?? []).filter((pod) => pod.namespace === selectedNamespace).map((pod) => pod.name);
    }
    return (options?.workloads ?? [])
      .filter((workload) => workload.namespace === selectedNamespace && workload.controller_type === selectedControllerType)
      .map((workload) => workload.name);
  }, [options, selectedControllerType, selectedNamespace]);

  const containerNameOptions = useMemo(() => {
    if (!selectedNamespace || !selectedControllerType || !selectedControllerName) {
      return [];
    }
    const containers = options?.containers ?? [];
    return containers
      .filter((container) => {
        if (container.namespace !== selectedNamespace) {
          return false;
        }
        if (container.controller_type || container.controller_name) {
          return container.controller_type === selectedControllerType && container.controller_name === selectedControllerName;
        }
        return selectedControllerType === 'pod' && container.pod === selectedControllerName;
      })
      .map((container) => container.name);
  }, [options, selectedControllerName, selectedControllerType, selectedNamespace]);

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
    setPolicy((current) => ({ ...current, [key]: value }));
  };

  const updateScope = (patch: Partial<Policy>) => {
    setPolicy((current) => ({ ...current, ...patch }));
  };

  const updateLogType = (value: string) => {
    if (value === 'host_file') {
      updateScope({
        log_type: value,
        namespace: '',
        controller_type: '',
        controller_name: '',
        container_name: '',
      });
      return;
    }
    updateScope({
      log_type: value,
      controller_type: policy.controller_type || '',
    });
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
            <div className={s.eyebrow}>
              Filebeat Ops / Policies /{' '}
              {isEdit
                ? t('filebeat-k8s-app.policyForm.editBreadcrumb', 'Edit policy')
                : t('filebeat-k8s-app.policyForm.createBreadcrumb', 'Create policy')}
            </div>
            <h1 className={s.title}>{t('filebeat-k8s-app.policyForm.title', 'Policy Create / Edit')}</h1>
            <div className={s.subtitle}>
              {t('filebeat-k8s-app.policyForm.subtitle', 'Use Kubernetes options to fill scope and preview rendered_config before saving.')}
            </div>
          </div>
          <div className={s.toolbar}>
            <LinkButton variant="secondary" href={prefixRoute(ROUTES.Policies)}>
              {t('filebeat-k8s-app.common.cancel', 'Cancel')}
            </LinkButton>
            <Button icon="save" onClick={onSave}>
              {t('filebeat-k8s-app.common.savePolicy', 'Save policy')}
            </Button>
          </div>
        </div>

        {error && (
          <Alert title={t('filebeat-k8s-app.common.saveFailed', 'Save failed')} severity="error">
            {error}
          </Alert>
        )}
        {options?.degraded && (
          <Alert title={t('filebeat-k8s-app.policyForm.kubernetesOptionsDegraded', 'Kubernetes options degraded')} severity="warning">
            {options.message}
          </Alert>
        )}

        <div className={s.split}>
          <section className={s.card}>
            <h2>{t('filebeat-k8s-app.policyForm.basicInfo', 'Basic information')}</h2>
            <div className={s.formGrid}>
              <Field label="id" value={policy.id} onChange={(value) => update('id', value)} placeholder="payment-app" disabled={isEdit} />
              <Field label="name" value={policy.name} onChange={(value) => update('name', value)} placeholder="payment app" />
              <Field label="cluster_id" value={policy.cluster_id} onChange={(value) => update('cluster_id', value)} placeholder="dev" />
              <SelectField
                label="namespace"
                value={policy.namespace || ''}
                options={namespaceOptions}
                onChange={(value) => updateScope({ namespace: value, controller_type: '', controller_name: '', container_name: '' })}
                disabled={isHostFile}
                placeholder={
                  isHostFile
                    ? t('filebeat-k8s-app.policyForm.hostFileNoNamespace', 'host_file does not need namespace')
                    : t('filebeat-k8s-app.policyForm.selectNamespace', 'Select namespace')
                }
              />
              <SelectField
                label="controller_type"
                value={policy.controller_type || ''}
                options={availableControllerTypes}
                onChange={(value) => updateScope({ controller_type: value, controller_name: '', container_name: '' })}
                disabled={isHostFile || !selectedNamespace}
                placeholder={
                  !selectedNamespace
                    ? t('filebeat-k8s-app.policyForm.chooseNamespaceFirst', 'Choose namespace first')
                    : t('filebeat-k8s-app.policyForm.selectControllerType', 'Select controller_type')
                }
              />
              <SelectField
                label="controller_name"
                value={policy.controller_name || ''}
                options={controllerNameOptions}
                onChange={(value) => updateScope({ controller_name: value, container_name: '' })}
                disabled={isHostFile || !selectedNamespace || !selectedControllerType}
                placeholder={
                  !selectedNamespace
                    ? t('filebeat-k8s-app.policyForm.chooseNamespaceFirst', 'Choose namespace first')
                    : !selectedControllerType
                      ? t('filebeat-k8s-app.policyForm.chooseControllerTypeFirst', 'Choose controller_type first')
                      : t('filebeat-k8s-app.policyForm.selectControllerName', 'Select controller_name')
                }
              />
              <SelectField
                label="container_name"
                value={policy.container_name || ''}
                options={containerNameOptions}
                onChange={(value) => update('container_name', value)}
                disabled={isHostFile || !selectedControllerName}
                placeholder={
                  !selectedControllerName
                    ? t('filebeat-k8s-app.policyForm.chooseControllerNameFirst', 'Choose controller_name first')
                    : t('filebeat-k8s-app.policyForm.selectContainerName', 'Select container_name')
                }
              />
              <SelectField
                label="log_type"
                value={policy.log_type}
                options={logTypeOptions}
                onChange={updateLogType}
                placeholder={t('filebeat-k8s-app.policyForm.selectLogType', 'Select log_type')}
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

            <h2>{t('filebeat-k8s-app.policyForm.advancedMatch', 'Advanced matching')}</h2>
            <div className={s.formGrid}>
              <Field label="node_selector" value={policy.node_selector || ''} onChange={(value) => update('node_selector', value)} placeholder="nodepool=online,zone=hk" />
              <Field
                label="log_path"
                value={policy.log_path || ''}
                onChange={(value) => update('log_path', value)}
                placeholder={t('filebeat-k8s-app.policyForm.logPathPlaceholder', '/app/logs/*.log or /var/log/messages')}
              />
              <div className={`${s.field} ${s.fullSpan}`}>
                <label>custom_fields</label>
                <textarea
                  className={s.input}
                  rows={5}
                  value={customFields}
                  onChange={(event) => setCustomFields(event.target.value)}
                  placeholder={'__project__=cloudnet\n__logstore__=payment'}
                />
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
                <h2>{t('filebeat-k8s-app.policyForm.yamlPreview', 'YAML preview')}</h2>
                <div className={`${s.muted} ${s.mono}`}>
                  {renderedChecksum || t('filebeat-k8s-app.policyForm.checksumPending', 'rendered_checksum pending')}
                </div>
              </div>
              <span className={`${s.chip} ${s.chipBlue}`}>render-preview</span>
            </div>
            {previewError && <div className={s.error}>{previewError}</div>}
            <pre className={s.code}>
              {renderedConfig || t('filebeat-k8s-app.policyForm.renderedConfigPending', 'Fill a valid policy to generate rendered_config automatically.')}
            </pre>
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
      <input
        className={s.input}
        value={value}
        placeholder={placeholder}
        disabled={disabled}
        onChange={(event: ChangeEvent<HTMLInputElement>) => onChange(event.target.value)}
      />
    </div>
  );
}

interface SelectFieldProps {
  label: string;
  value: string;
  options: string[];
  onChange: (value: string) => void;
  placeholder?: string;
  disabled?: boolean;
}

function SelectField({ label, value, options, onChange, placeholder, disabled }: SelectFieldProps) {
  const s = useStyles2(getPageStyles);
  const uniqueOptions = Array.from(new Set([...options, value].filter(Boolean))).sort();
  const selectOptions: Array<ComboboxOption<string>> = uniqueOptions.map((option) => ({ label: option, value: option }));
  return (
    <div className={s.field}>
      <label>{label}</label>
      <Combobox
        value={value || null}
        options={selectOptions}
        placeholder={placeholder}
        disabled={disabled}
        isClearable={!disabled}
        onChange={(selected: ComboboxOption<string> | null) => onChange(selected?.value ?? '')}
        width="auto"
        minWidth={20}
      />
    </div>
  );
}

export default PolicyFormPage;
