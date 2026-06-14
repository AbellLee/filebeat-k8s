import React, { ChangeEvent, useEffect, useMemo, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { PluginPage } from '@grafana/runtime';
import { Alert, Button, Combobox, ComboboxOption, IconButton, LinkButton, Popover, useStyles2 } from '@grafana/ui';
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
  const isFileLogType = policy.log_type === 'container_file' || policy.log_type === 'host_file';
  const logPathHint =
    policy.log_type === 'host_file'
      ? t('filebeat-k8s-app.policyForm.hints.hostLogPath', 'Absolute path on the node, for example /var/log/messages.')
      : t('filebeat-k8s-app.policyForm.hints.containerLogPath', 'Path inside the container, for example /app/logs/*.log.');
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
    const next = {
      ...normalizePolicyForLogType(policy),
      custom_fields: customFieldsFromText(customFields),
      input_config: parsedInputConfig.config,
    };
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
    if (value === 'container_stdio') {
      updateScope({
        log_type: value,
        log_path: '',
        controller_type: policy.controller_type || '',
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
    const payload = {
      ...normalizePolicyForLogType(policy),
      custom_fields: customFieldsFromText(customFields),
      input_config: parsedInputConfig.config,
    };
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
              <Field
                label={t('filebeat-k8s-app.fields.id', 'ID')}
                value={policy.id}
                onChange={(value) => update('id', value)}
                placeholder="payment-app"
                disabled={isEdit}
                hint={t('filebeat-k8s-app.policyForm.hints.id', 'Stable policy ID. Use lowercase letters, numbers, and dashes; it cannot be changed after creation.')}
              />
              <Field
                label={t('filebeat-k8s-app.fields.name', 'Name')}
                value={policy.name}
                onChange={(value) => update('name', value)}
                placeholder="payment app"
                hint={t('filebeat-k8s-app.policyForm.hints.name', 'Human-readable name shown in policy lists.')}
              />
              <Field
                label={t('filebeat-k8s-app.fields.clusterId', 'Cluster ID')}
                value={policy.cluster_id}
                onChange={(value) => update('cluster_id', value)}
                placeholder="dev"
                hint={t('filebeat-k8s-app.policyForm.hints.clusterId', 'Must match the cluster_id reported by sidecar Agents, for example dev or prod-hk.')}
              />
              <SelectField
                label={t('filebeat-k8s-app.fields.namespace', 'Namespace')}
                value={policy.namespace || ''}
                options={namespaceOptions}
                onChange={(value) => updateScope({ namespace: value, controller_type: '', controller_name: '', container_name: '' })}
                disabled={isHostFile}
                hint={t('filebeat-k8s-app.policyForm.hints.namespace', 'Kubernetes namespace that contains the workload. Select it first to unlock controller choices.')}
                placeholder={
                  isHostFile
                    ? t('filebeat-k8s-app.policyForm.hostFileNoNamespace', 'host_file does not need namespace')
                    : t('filebeat-k8s-app.policyForm.selectNamespace', 'Select namespace')
                }
              />
              <SelectField
                label={t('filebeat-k8s-app.fields.controllerType', 'Controller type')}
                value={policy.controller_type || ''}
                options={availableControllerTypes}
                onChange={(value) => updateScope({ controller_type: value, controller_name: '', container_name: '' })}
                disabled={isHostFile || !selectedNamespace}
                hint={t('filebeat-k8s-app.policyForm.hints.controllerType', 'Workload type. Use pod for a single Pod, or deployment/statefulset/daemonset/job/cronjob for controllers.')}
                placeholder={
                  !selectedNamespace
                    ? t('filebeat-k8s-app.policyForm.chooseNamespaceFirst', 'Choose namespace first')
                    : t('filebeat-k8s-app.policyForm.selectControllerType', 'Select controller_type')
                }
              />
              <SelectField
                label={t('filebeat-k8s-app.fields.controllerName', 'Controller name')}
                value={policy.controller_name || ''}
                options={controllerNameOptions}
                onChange={(value) => updateScope({ controller_name: value, container_name: '' })}
                disabled={isHostFile || !selectedNamespace || !selectedControllerType}
                hint={t('filebeat-k8s-app.policyForm.hints.controllerName', 'Workload or Pod name under the selected namespace and controller type.')}
                placeholder={
                  !selectedNamespace
                    ? t('filebeat-k8s-app.policyForm.chooseNamespaceFirst', 'Choose namespace first')
                    : !selectedControllerType
                      ? t('filebeat-k8s-app.policyForm.chooseControllerTypeFirst', 'Choose controller_type first')
                      : t('filebeat-k8s-app.policyForm.selectControllerName', 'Select controller_name')
                }
              />
              <SelectField
                label={t('filebeat-k8s-app.fields.containerName', 'Container name')}
                value={policy.container_name || ''}
                options={containerNameOptions}
                onChange={(value) => update('container_name', value)}
                disabled={isHostFile || !selectedControllerName}
                hint={t('filebeat-k8s-app.policyForm.hints.containerName', 'Container whose logs should be collected.')}
                placeholder={
                  !selectedControllerName
                    ? t('filebeat-k8s-app.policyForm.chooseControllerNameFirst', 'Choose controller_name first')
                    : t('filebeat-k8s-app.policyForm.selectContainerName', 'Select container_name')
                }
              />
              <SelectField
                label={t('filebeat-k8s-app.fields.logType', 'Log type')}
                value={policy.log_type}
                options={logTypeOptions}
                onChange={updateLogType}
                hint={t('filebeat-k8s-app.policyForm.hints.logType', 'container_stdio collects stdout/stderr; container_file collects files inside the container; host_file collects node files.')}
                placeholder={t('filebeat-k8s-app.policyForm.selectLogType', 'Select log_type')}
              />
              <Field
                label={t('filebeat-k8s-app.fields.priority', 'Priority')}
                value={String(policy.priority)}
                onChange={(value) => update('priority', Number(value) || 0)}
                hint={t('filebeat-k8s-app.policyForm.hints.priority', 'Higher priority wins when multiple enabled policies match the same target.')}
              />
              <div className={s.field}>
                <FieldLabel
                  label={t('filebeat-k8s-app.fields.enabled', 'Enabled')}
                  hint={t('filebeat-k8s-app.policyForm.hints.enabled', 'Disabled policies are saved but not rendered to Agents.')}
                />
                <div className={s.checkboxLine}>
                  <input type="checkbox" checked={policy.enabled} onChange={(event) => update('enabled', event.currentTarget.checked)} />
                  <span>{policy.enabled ? t('filebeat-k8s-app.status.enabled', 'enabled') : t('filebeat-k8s-app.status.disabled', 'disabled')}</span>
                </div>
              </div>
            </div>

            <h2>{t('filebeat-k8s-app.policyForm.advancedMatch', 'Advanced matching')}</h2>
            <div className={s.formGrid}>
              <Field
                label={t('filebeat-k8s-app.fields.nodeSelector', 'Node selector')}
                value={policy.node_selector || ''}
                onChange={(value) => update('node_selector', value)}
                placeholder="nodepool=online,zone=hk"
                hint={t('filebeat-k8s-app.policyForm.hints.nodeSelector', 'Optional comma-separated key=value pairs. Leave empty to match all nodes.')}
              />
              {isFileLogType && (
                <Field
                  label={t('filebeat-k8s-app.fields.logPath', 'Log path')}
                  value={policy.log_path || ''}
                  onChange={(value) => update('log_path', value)}
                  placeholder={t('filebeat-k8s-app.policyForm.logPathPlaceholder', '/app/logs/*.log or /var/log/messages')}
                  hint={logPathHint}
                />
              )}
              <div className={`${s.field} ${s.fullSpan}`}>
                <FieldLabel
                  label={t('filebeat-k8s-app.fields.customFields', 'Custom fields')}
                  hint={t('filebeat-k8s-app.policyForm.hints.customFields', 'One key=value pair per line. These fields are added to each event by Filebeat add_fields.')}
                />
                <textarea
                  className={s.input}
                  rows={5}
                  value={customFields}
                  onChange={(event) => setCustomFields(event.target.value)}
                  placeholder={'__project__=cloudnet\n__logstore__=payment'}
                />
              </div>
              <div className={`${s.field} ${s.fullSpan}`}>
                <FieldLabel
                  label={t('filebeat-k8s-app.fields.inputConfig', 'Input config')}
                  hint={t(
                    'filebeat-k8s-app.policyForm.hints.inputConfig',
                    'YAML object merged into the Filebeat filestream input. Reserved fields such as paths, parsers, and processors cannot be overridden.'
                  )}
                  hintMode="popover"
                />
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

function normalizePolicyForLogType(policy: Policy): Policy {
  if (policy.log_type === 'container_stdio') {
    return { ...policy, log_path: '' };
  }
  return policy;
}

interface FieldLabelProps {
  label: string;
  hint?: string;
  hintMode?: 'tooltip' | 'popover';
}

function FieldLabel({ label, hint, hintMode = 'tooltip' }: FieldLabelProps) {
  const s = useStyles2(getPageStyles);

  return (
    <div className={s.fieldLabel}>
      <span>{label}</span>
      {hint && hintMode === 'tooltip' && (
        <IconButton
          className={s.helpIcon}
          name="info-circle"
          size="sm"
          tooltip={hint}
          tooltipPlacement="top"
          type="button"
          variant="secondary"
        />
      )}
      {hint && hintMode === 'popover' && <FieldHelpPopover content={hint} />}
    </div>
  );
}

interface FieldHelpPopoverProps {
  content: string;
}

function FieldHelpPopover({ content }: FieldHelpPopoverProps) {
  const s = useStyles2(getPageStyles);
  const [anchor, setAnchor] = useState<HTMLButtonElement | null>(null);
  const [show, setShow] = useState(false);

  return (
    <>
      <IconButton
        ref={setAnchor}
        className={s.helpIcon}
        name="info-circle"
        size="sm"
        aria-label={content}
        onBlur={() => setShow(false)}
        onClick={() => setShow((current) => !current)}
        onKeyDown={(event) => {
          if (event.key === 'Escape') {
            setShow(false);
          }
        }}
        type="button"
        variant="secondary"
      />
      {anchor && (
        <Popover
          content={<div className={s.helpPopover}>{content}</div>}
          hidePopper={() => setShow(false)}
          placement="top"
          referenceElement={anchor}
          renderArrow
          show={show}
        />
      )}
    </>
  );
}

interface FieldProps {
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  disabled?: boolean;
  hint?: string;
}

function Field({ label, value, onChange, placeholder, disabled, hint }: FieldProps) {
  const s = useStyles2(getPageStyles);
  return (
    <div className={s.field}>
      <FieldLabel label={label} hint={hint} />
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
  hint?: string;
}

function SelectField({ label, value, options, onChange, placeholder, disabled, hint }: SelectFieldProps) {
  const s = useStyles2(getPageStyles);
  const uniqueOptions = Array.from(new Set([...options, value].filter(Boolean))).sort();
  const selectOptions: Array<ComboboxOption<string>> = uniqueOptions.map((option) => ({ label: option, value: option }));
  return (
    <div className={s.field}>
      <FieldLabel label={label} hint={hint} />
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
